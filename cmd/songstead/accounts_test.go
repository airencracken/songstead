// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/airencracken/comfylib/svcconfig"
	"github.com/airencracken/songstead/internal/store"
)

func TestCLICreateOwnerAndListUsers(t *testing.T) {
	data := privateDir(t)
	var out bytes.Buffer
	args := []string{"create-owner", "--data-dir", data, "--username", "alex", "--password-stdin"}
	if err := run(t.Context(), args, strings.NewReader("a-long-owner-password\r\n"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Created owner: alex") || strings.Contains(out.String(), "a-long-owner-password") {
		t.Fatal("owner output contract", out.String())
	}
	s, err := store.Open(filepath.Join(data, "songstead.db"))
	if err != nil {
		t.Fatal(err)
	}
	u, hash, err := s.Credentials(t.Context(), "alex")
	if err != nil || u.Role != "owner" {
		t.Fatal("CLI did not create owner", u, err)
	}
	secret, err := s.NewSession(t.Context(), u.ID, hash)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	args[4] = "ALEX"
	if err := run(t.Context(), args, strings.NewReader("a-different-password"), &out); err == nil {
		t.Fatal("CLI overwrote existing account")
	}
	s, err = store.Open(filepath.Join(data, "songstead.db"))
	if err != nil {
		t.Fatal(err)
	}
	if current, currentHash, err := s.Credentials(t.Context(), "alex"); err != nil || current != u || currentHash != hash {
		t.Fatal("duplicate provisioning was not atomic", err)
	}
	if _, err := s.Session(t.Context(), secret); err != nil {
		t.Fatal("duplicate provisioning revoked session", err)
	}
	s.Close()
	out.Reset()
	if err := run(t.Context(), []string{"list-users", "--data-dir", data}, nil, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ID", "USERNAME", "ROLE", "alex", "owner"} {
		if !strings.Contains(out.String(), want) {
			t.Fatal("list-users missing field", want, out.String())
		}
	}
	if strings.Contains(out.String(), hash) || strings.Contains(out.String(), "a-long-owner-password") {
		t.Fatal("list-users exposed credentials")
	}
	backupPath := filepath.Join(t.TempDir(), "backup.db")
	if err := run(t.Context(), []string{"backup", "--data-dir", data, "--output", backupPath}, nil, &out); err != nil {
		t.Fatal(err)
	}
	restored := privateDir(t)
	if err := run(t.Context(), []string{"restore", "--data-dir", restored, "--input", backupPath}, nil, &out); err != nil {
		t.Fatal(err)
	}
	s, err = store.Open(filepath.Join(restored, "songstead.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if current, currentHash, err := s.Credentials(t.Context(), "alex"); err != nil || current.Role != "owner" || currentHash != hash {
		t.Fatal("restore lost owner", current, err)
	}
}

func TestCLIAccountServiceConfiguration(t *testing.T) {
	config := filepath.Join(t.TempDir(), "songstead.confd")
	serviceDir := filepath.Join(privateDir(t), "service-data")
	if err := os.WriteFile(config, []byte("SONGSTEAD_DATA_DIR=\""+serviceDir+"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	paths := svcconfig.Paths{Name: "Songstead", Prefix: "SONGSTEAD_", OpenRCConfig: config, OpenRCInstalled: true, OpenRCActive: true, DefaultDataDir: "/var/lib/songstead"}
	t.Setenv("SONGSTEAD_DATA_DIR", "")
	var out bytes.Buffer
	args := []string{"create-owner", "--username", "alex", "--password-stdin"}
	if err := runWithPaths(t.Context(), args, strings.NewReader("a-long-owner-password"), &out, paths); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(serviceDir, "songstead.db")); err != nil {
		t.Fatal("CLI ignored installed service", err)
	}
	envDir := privateDir(t)
	t.Setenv("SONGSTEAD_DATA_DIR", envDir)
	if err := runWithPaths(t.Context(), args, strings.NewReader("a-long-owner-password"), &out, paths); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(envDir, "songstead.db")); err != nil {
		t.Fatal("environment override ignored", err)
	}
	flagDir := privateDir(t)
	args = append(args, "--data-dir", flagDir)
	if err := runWithPaths(t.Context(), args, strings.NewReader("a-long-owner-password"), &out, paths); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(flagDir, "songstead.db")); err != nil {
		t.Fatal("CLI data directory override ignored", err)
	}
	request := provisioningRequest(args, paths)
	if request.DefaultUser != "songstead" || !reflect.DeepEqual(request.Paths, paths) || !request.Commands["create-owner"] || !request.Commands["create-user"] || !request.Commands["list-users"] || request.Commands["serve"] || request.Commands["restore"] {
		t.Fatal("service privilege request includes wrong commands")
	}
}

func TestCLIHelpAndFlagContracts(t *testing.T) {
	for _, args := range [][]string{
		{"help", "create-owner"}, {"create-owner", "--help"}, {"create-user", "--help"},
		{"set-password", "--help"}, {"list-users", "--help"}, {"help", "version"},
	} {
		var out bytes.Buffer
		if err := run(t.Context(), args, nil, &out); err != nil || !strings.Contains(out.String(), "Usage: songstead ") {
			t.Fatal("command help contract", args, err, out.String())
		}
	}
	for _, args := range [][]string{
		{"--help", "extra"}, {"--version", "extra"}, {"version", "extra"}, {"help", "unknown"},
		{"create-owner"}, {"create-owner", "--username", "alex"},
		{"create-owner", "--username", "alex", "--password-prompt", "--password-stdin"},
		{"create-owner", "--username", "alex", "--password", "a-long-owner-password"},
		{"create-owner", "--username", "alex", "--password-stdin", "--addr", "127.0.0.1:8083"},
		{"list-users", "--password-stdin"}, {"serve", "--username", "alex"}, {"list-users", "extra"},
	} {
		if err := run(t.Context(), args, strings.NewReader("a-long-owner-password"), &bytes.Buffer{}); err == nil {
			t.Fatal("invalid CLI accepted", args)
		}
	}
}

func TestCLIAccountFailuresLeaveNoDatabase(t *testing.T) {
	for _, command := range []string{"create-owner", "create-user", "set-password"} {
		for _, password := range []string{"short", "a-long-owner-password\nsecond-line", strings.Repeat("x", 73)} {
			data := filepath.Join(privateDir(t), "new")
			args := []string{command, "--data-dir", data, "--username", "alex", "--password-stdin"}
			if err := run(t.Context(), args, strings.NewReader(password), &bytes.Buffer{}); err == nil {
				t.Fatal("invalid password accepted", command)
			}
			if _, err := os.Stat(data); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("invalid credential input created data directory", err)
			}
		}
	}
	data := filepath.Join(privateDir(t), "new")
	args := []string{"create-owner", "--data-dir", data, "--username", "alex", "--password-prompt"}
	if err := run(t.Context(), args, strings.NewReader("a-long-owner-password"), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "requires a terminal") {
		t.Fatal("nonterminal prompt accepted", err)
	}
	if _, err := os.Stat(data); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed prompt created database", err)
	}
	for _, command := range []string{"list-users", "set-password", "backup"} {
		args := []string{command, "--data-dir", data}
		if command == "set-password" {
			args = append(args, "--username", "alex", "--password-stdin")
		}
		if command == "backup" {
			args = append(args, "--output", filepath.Join(privateDir(t), "backup.db"))
		}
		if err := run(t.Context(), args, strings.NewReader("a-long-owner-password"), &bytes.Buffer{}); err == nil {
			t.Fatal("command silently created an empty instance", command)
		}
		if _, err := os.Stat(data); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("missing instance command created database", command, err)
		}
	}
}

func TestCLIRefusesRootAccountWrites(t *testing.T) {
	old := geteuid
	geteuid = func() int { return 0 }
	t.Cleanup(func() { geteuid = old })
	data := filepath.Join(privateDir(t), "new")
	args := []string{"create-owner", "--data-dir", data, "--username", "alex", "--password-stdin"}
	if err := run(t.Context(), args, strings.NewReader("a-long-owner-password"), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "must not run as root") {
		t.Fatal("root account write accepted", err)
	}
	if _, err := os.Stat(data); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("root refusal created database", err)
	}
	if err := run(t.Context(), []string{"create-owner", "--help"}, nil, &bytes.Buffer{}); err != nil {
		t.Fatal("root help was refused", err)
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestConfirmedPassword(t *testing.T) {
	for _, tc := range []struct {
		first, second string
		valid         bool
	}{{"a-long-owner-password", "a-long-owner-password", true}, {"first-long-password", "second-long-password", false}, {"short", "short", false}, {"  a-long-password  ", "  a-long-password  ", true}} {
		var out bytes.Buffer
		reads := 0
		password, err := readConfirmedPassword(&out, func() ([]byte, error) {
			reads++
			if reads == 1 {
				return []byte(tc.first), nil
			}
			return []byte(tc.second), nil
		})
		if (err == nil) != tc.valid || tc.valid && password != tc.first {
			t.Fatal("confirmed password contract", tc.valid, err)
		}
		if out.String() != "Password: \nConfirm password: \n" {
			t.Fatal("prompt printed password", out.String())
		}
	}
	if _, err := readConfirmedPassword(failedWriter{}, func() ([]byte, error) { t.Fatal("read despite output failure"); return nil, nil }); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("prompt output error lost", err)
	}
	if _, err := readConfirmedPassword(&bytes.Buffer{}, func() ([]byte, error) { return nil, io.ErrUnexpectedEOF }); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("prompt input error lost", err)
	}
}
