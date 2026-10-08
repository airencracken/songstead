// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/airencracken/comfylib/clientip"
	"github.com/airencracken/comfylib/privdrop"
	"github.com/airencracken/comfylib/svcconfig"
	"github.com/airencracken/songstead/internal/media"
	"github.com/airencracken/songstead/internal/store"
	"github.com/airencracken/songstead/internal/web"
	"github.com/gofrs/flock"
)

var version = "development"

func main() {
	if handled, status, err := privdrop.Reexec(provisioningRequest(os.Args[1:], servicePaths())); handled {
		if err != nil {
			slog.Error("songstead", "error", err)
		}
		os.Exit(status)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout); err != nil {
		slog.Error("songstead", "error", err)
		os.Exit(1)
	}
}

const help = `Songstead: good music, from your people.

Usage:
  songstead serve [--addr 127.0.0.1:8083] [--data-dir ./data]
  songstead sandbox [--check] [--data-dir ./data] [--bwrap bwrap]
  songstead create-owner --username NAME (--password-prompt | --password-stdin) [--data-dir DIRECTORY]
  songstead create-user --username NAME (--password-prompt | --password-stdin) [--data-dir DIRECTORY]
  songstead set-password --username NAME (--password-prompt | --password-stdin) [--data-dir DIRECTORY]
  songstead list-users [--data-dir DIRECTORY]
  songstead backup --output FILE [--data-dir ./data]
  songstead restore --input FILE [--data-dir NEW_DIRECTORY]
  songstead --version
  songstead help [COMMAND]

With no command, starts the server. Accounts are created locally; registration is closed.
Password commands use a hidden, confirmed terminal prompt or read one line from stdin.
create-owner creates a new owner; existing accounts are never promoted or changed.
set-password revokes all sessions. list-users prints account IDs, names, and roles.
Account and backup commands discover the installed service's data directory unless
--data-dir or SONGSTEAD_DATA_DIR is set. Root invocations run as the service user.
Service settings: /etc/conf.d/songstead (OpenRC), /etc/songstead/songstead.env (systemd).
Environment: SONGSTEAD_DATA_DIR, SONGSTEAD_ADDR, SONGSTEAD_SECURE_COOKIES,
SONGSTEAD_TRUSTED_PROXIES (comma-separated proxy IPs or CIDRs),
SONGSTEAD_BASE_URL, SONGSTEAD_WITMOOT_URL (optional discussion handoffs), SONGSTEAD_BWRAP.
`

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func run(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	return runWithPaths(ctx, args, in, out, servicePaths())
}

func runWithPaths(ctx context.Context, args []string, in io.Reader, out io.Writer, paths svcconfig.Paths) error {
	command := "serve"
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
	}
	if command == "--version" || command == "version" {
		if command == "version" && len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
			_, err := io.WriteString(out, "Usage: songstead version\nPrint the running build version.\n")
			return err
		}
		if len(args) != 0 {
			return errors.New("usage: songstead version (also --version)")
		}
		_, err := fmt.Fprintln(out, "songstead", version)
		return err
	}
	if command == "help" || command == "--help" || command == "-h" {
		if command == "help" && len(args) == 1 && (commandDescriptions[args[0]] != "" || args[0] == "sandbox" || args[0] == "version") {
			return runWithPaths(ctx, []string{args[0], "--help"}, in, out, paths)
		}
		if len(args) != 0 {
			return errors.New("usage: songstead help [COMMAND]")
		}
		_, err := io.WriteString(out, help)
		return err
	}
	if command == "sandbox" {
		return runSandbox(ctx, args, out)
	}
	if commandDescriptions[command] == "" {
		return fmt.Errorf("unknown command %q; use --help", command)
	}
	flags := commandFlags(command, out)
	data := flags.String("data-dir", "", "private data directory (environment, then installed service, then ./data)")
	var addr, name, output, input string
	var stdin, prompt bool
	switch command {
	case "serve":
		flags.StringVar(&addr, "addr", envDefault("SONGSTEAD_ADDR", "127.0.0.1:8083"), "listen address")
	case "create-owner", "create-user", "set-password":
		flags.StringVar(&name, "username", "", "account username (required)")
		flags.BoolVar(&stdin, "password-stdin", false, "read one password line from standard input")
		flags.BoolVar(&prompt, "password-prompt", false, "prompt twice without echoing (requires a terminal)")
	case "backup":
		flags.StringVar(&output, "output", "", "backup destination (must not exist)")
	case "restore":
		flags.StringVar(&input, "input", "", "snapshot to restore into an empty data directory")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	account := command == "create-owner" || command == "create-user" || command == "set-password"
	if account && (stdin == prompt || name == "") {
		return errors.New("provide --username and exactly one of --password-prompt or --password-stdin")
	}
	if command == "backup" && output == "" {
		return errors.New("provide --output")
	}
	if command == "restore" && input == "" {
		return errors.New("provide --input")
	}
	if provisioningCommands[command] {
		if err := privdrop.RefuseRoot(geteuid(), command, "sudo -u songstead env SONGSTEAD_DATA_DIR=/var/lib/songstead songstead "+command); err != nil {
			return err
		}
	}
	var password string
	if account {
		if err := store.ValidateUsername(name); err != nil {
			return err
		}
		var err error
		if prompt {
			password, err = promptPassword(in, out)
		} else {
			password, err = readPassword(in)
		}
		if err != nil {
			return err
		}
	}
	if err := resolveDataDir(flags, data, paths, provisioningCommands[command]); err != nil {
		return err
	}
	path := filepath.Join(*data, "songstead.db")
	if command == "set-password" || command == "list-users" || command == "backup" {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("open existing Songstead database: %w", err)
		}
	}
	if err := os.MkdirAll(*data, 0700); err != nil {
		return err
	}
	info, err := os.Stat(*data)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0077 != 0 {
		return errors.New("data directory must be private: chmod 700 it before continuing")
	}
	if command == "restore" {
		return restore(ctx, input, path)
	}
	if command == "serve" {
		lock := flock.New(filepath.Join(*data, ".server.lock"))
		locked, err := lock.TryLock()
		if err != nil {
			return err
		}
		if !locked {
			return errors.New("another Songstead server is using this data directory")
		}
		defer lock.Close()
	}
	open := store.Open
	if command != "serve" {
		if _, err := os.Stat(path); err == nil {
			open = store.OpenCurrent
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	s, err := open(path)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := os.Chmod(path, 0600); err != nil {
		return err
	}
	switch command {
	case "create-owner", "create-user", "set-password":
		if command == "create-user" {
			_, err = s.CreateUser(ctx, name, password)
		} else if command == "create-owner" {
			_, err = s.CreateOwner(ctx, name, password)
		} else {
			err = s.SetPassword(ctx, name, password)
		}
		if err != nil {
			return err
		}
		if command == "create-owner" {
			_, err = fmt.Fprintln(out, "Created owner:", name)
		} else {
			_, err = fmt.Fprintln(out, "Account updated:", name)
		}
		return err
	case "list-users":
		return printUsers(ctx, s, out)
	case "backup":
		return backup(ctx, s, output)
	default:
		return serve(ctx, s, addr)
	}
}

func readPassword(in io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(in, 4096))
	if err != nil {
		return "", err
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if strings.ContainsAny(password, "\r\n") {
		return "", errors.New("provide exactly one password line")
	}
	if err := store.ValidatePassword(password); err != nil {
		return "", err
	}
	return password, nil
}

func serve(ctx context.Context, s *store.Store, addr string) error {
	secure := false
	if value := os.Getenv("SONGSTEAD_SECURE_COOKIES"); value != "" {
		var err error
		secure, err = strconv.ParseBool(value)
		if err != nil {
			return errors.New("SONGSTEAD_SECURE_COOKIES must be a boolean")
		}
	}
	trusted, err := clientip.ParseTrusted(os.Getenv("SONGSTEAD_TRUSTED_PROXIES"), "SONGSTEAD_TRUSTED_PROXIES")
	if err != nil {
		return err
	}
	a, err := web.New(s, web.Config{WitmootURL: os.Getenv("SONGSTEAD_WITMOOT_URL"), BaseURL: os.Getenv("SONGSTEAD_BASE_URL"), SecureCookies: secure, TrustedProxies: trusted})
	if err != nil {
		return err
	}
	server := &http.Server{Addr: addr, Handler: a, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	workerCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); metadataWorker(workerCtx, s) }()
	defer func() { cancel(); wg.Wait() }()
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	slog.Info("Songstead listening", "address", addr)
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return err
		}
		err := <-done
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func metadataWorker(ctx context.Context, s *store.Store) {
	client := media.Client()
	defer client.CloseIdleConnections()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := s.Metadata(ctx, func(ctx context.Context, id string) (media.Metadata, error) { return media.YouTube(ctx, client, id) })
			if err != nil && ctx.Err() == nil {
				slog.Error("metadata worker", "error", err)
			}
		}
	}
}

func backup(ctx context.Context, s *store.Store, output string) error {
	tmp, err := os.CreateTemp(filepath.Dir(output), ".songstead-backup-*")
	if err != nil {
		return err
	}
	path := tmp.Name()
	defer os.Remove(path)
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	if err := s.Backup(ctx, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0600); err != nil {
		return err
	}
	if err := store.ValidateSnapshot(ctx, path); err != nil {
		return err
	}
	return os.Link(path, output)
}

func restore(ctx context.Context, input, target string) error {
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("restore requires an empty data directory")
	}
	if _, err := os.Lstat(target); err == nil {
		return errors.New("restore requires a new data directory without songstead.db")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	src, err := os.Open(input)
	if err != nil {
		return err
	}
	defer src.Close()
	tmp, err := os.CreateTemp(filepath.Dir(target), ".songstead-restore-*")
	if err != nil {
		return err
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := store.ValidateSnapshot(ctx, path); err != nil {
		return err
	}
	return os.Link(path, target)
}
