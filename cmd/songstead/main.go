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
	"github.com/airencracken/songstead/internal/media"
	"github.com/airencracken/songstead/internal/store"
	"github.com/airencracken/songstead/internal/web"
	"github.com/gofrs/flock"
)

var version = "development"

func main() {
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
  songstead create-user --username NAME --password-stdin [--data-dir ./data]
  songstead set-password --username NAME --password-stdin [--data-dir ./data]
  songstead backup --output FILE [--data-dir ./data]
  songstead restore --input FILE [--data-dir NEW_DIRECTORY]
  songstead --version

With no command, starts the server. Accounts are created locally; registration is closed.
Password commands read one line from standard input. set-password revokes all sessions.
Environment: SONGSTEAD_DATA_DIR, SONGSTEAD_ADDR, SONGSTEAD_SECURE_COOKIES,
SONGSTEAD_TRUSTED_PROXIES (comma-separated proxy IPs or CIDRs).
`

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func run(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	command := "serve"
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
	}
	if command == "--version" || command == "version" {
		_, err := fmt.Fprintln(out, "songstead", version)
		return err
	}
	if command == "help" || command == "--help" || command == "-h" {
		_, err := io.WriteString(out, help)
		return err
	}
	if command != "serve" && command != "create-user" && command != "set-password" && command != "backup" && command != "restore" {
		return fmt.Errorf("unknown command %q; use --help", command)
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(out)
	data := flags.String("data-dir", envDefault("SONGSTEAD_DATA_DIR", "./data"), "private data directory")
	addr := flags.String("addr", envDefault("SONGSTEAD_ADDR", "127.0.0.1:8083"), "listen address")
	name := flags.String("username", "", "account username")
	stdin := flags.Bool("password-stdin", false, "read one password line from standard input")
	output := flags.String("output", "", "backup destination (must not exist)")
	input := flags.String("input", "", "snapshot to restore into an empty data directory")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	if (command == "create-user" || command == "set-password") && (!*stdin || *name == "") {
		return errors.New("provide --username and --password-stdin")
	}
	if command == "backup" && *output == "" {
		return errors.New("provide --output")
	}
	if command == "restore" && *input == "" {
		return errors.New("provide --input")
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
	path := filepath.Join(*data, "songstead.db")
	if command == "restore" {
		return restore(ctx, *input, path)
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
	s, err := store.Open(path)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := os.Chmod(path, 0600); err != nil {
		return err
	}
	switch command {
	case "create-user", "set-password":
		password, err := readPassword(in)
		if err != nil {
			return err
		}
		if command == "create-user" {
			_, err = s.CreateUser(ctx, *name, password)
		} else {
			err = s.SetPassword(ctx, *name, password)
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "Account updated:", *name)
		return err
	case "backup":
		return backup(ctx, s, *output)
	default:
		return serve(ctx, s, *addr)
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
