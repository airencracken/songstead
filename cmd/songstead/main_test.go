// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/airencracken/songstead/internal/store"
)

func TestCLIProvisionBackupRestore(t *testing.T) {
	ctx := context.Background()
	data := privateDir(t)
	var out bytes.Buffer
	if err := run(ctx, []string{"create-user", "--data-dir", data, "--username", "alice", "--password-stdin"}, strings.NewReader("a-long-test-password\n"), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "a-long-test-password") {
		t.Fatal("password printed")
	}
	s, err := store.Open(filepath.Join(data, "songstead.db"))
	if err != nil {
		t.Fatal(err)
	}
	u, hash, err := s.Credentials(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.NewSession(ctx, u.ID, hash)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := run(ctx, []string{"set-password", "--data-dir", data, "--username", "alice", "--password-stdin"}, strings.NewReader("a-new-long-password\n"), &out); err != nil {
		t.Fatal(err)
	}
	s, err = store.Open(filepath.Join(data, "songstead.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Session(ctx, session); err == nil {
		t.Fatal("password change did not revoke sessions")
	}
	s.Close()
	backup := filepath.Join(t.TempDir(), "backup.db")
	if err := run(ctx, []string{"backup", "--data-dir", data, "--output", backup}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(backup)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("backup permissions", err)
	}
	restored := privateDir(t)
	if err := run(ctx, []string{"restore", "--data-dir", restored, "--input", backup}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	s, err = store.Open(filepath.Join(restored, "songstead.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, _, err := s.Credentials(ctx, "alice"); err != nil {
		t.Fatal("restore lost account", err)
	}
	if err := run(ctx, []string{"restore", "--data-dir", restored, "--input", backup}, strings.NewReader(""), &out); err == nil {
		t.Fatal("restore overwrote existing database")
	}
}

func TestFailedRestoreIsAtomic(t *testing.T) {
	ctx := context.Background()
	data := privateDir(t)
	target := filepath.Join(data, "songstead.db")
	input := filepath.Join(t.TempDir(), "invalid.db")
	if err := os.WriteFile(input, []byte("not a SQLite database"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := restore(ctx, input, target); err == nil {
		t.Fatal("bad snapshot accepted")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("failed restore published a database")
	}
	files, err := os.ReadDir(data)
	if err != nil || len(files) != 0 {
		t.Fatal("temporary restore files left behind", files, err)
	}
}

func TestCLIContractsAndAdversarialInput(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	for _, args := range [][]string{{"--help"}, {"--version"}, {"serve", "--help"}} {
		if err := run(ctx, args, strings.NewReader(""), &out); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(out.String(), "songstead development") {
		t.Fatal("version missing")
	}
	for _, args := range [][]string{{"unknown"}, {"create-user", "--username", "alice"}, {"create-user", "--password-stdin"}, {"backup"}, {"restore"}, {"serve", "unexpected"}} {
		if err := run(ctx, args, strings.NewReader("a-long-test-password"), &out); err == nil {
			t.Fatal("bad arguments accepted", args)
		}
	}
	for _, input := range []string{"short", strings.Repeat("x", 73), "a-long-test-password\nanother-line", strings.Repeat("x", 4096)} {
		if _, err := readPassword(strings.NewReader(input)); err == nil {
			t.Fatal("bad password input accepted")
		}
	}
	password, err := readPassword(strings.NewReader("  a-long-password  \n"))
	if err != nil || password != "  a-long-password  " {
		t.Fatal("password whitespace lost")
	}
	data := privateDir(t)
	if err := os.Chmod(data, 0755); err != nil {
		t.Fatal(err)
	}
	if err := run(ctx, []string{"backup", "--data-dir", data, "--output", filepath.Join(data, "backup")}, strings.NewReader(""), &out); err == nil {
		t.Fatal("public data directory accepted")
	}
}

func privateDir(t *testing.T) string {
	t.Helper()
	p := t.TempDir()
	if err := os.Chmod(p, 0700); err != nil {
		t.Fatal(err)
	}
	return p
}
