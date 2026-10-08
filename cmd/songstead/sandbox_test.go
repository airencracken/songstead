// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func fakeBubblewrap(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	record := filepath.Join(dir, "arguments")
	fake := filepath.Join(dir, "bwrap")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + "'" + record + "'\n"
	if err := os.WriteFile(fake, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return fake, record
}

func TestSandboxCheckRunsTheBoundServer(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the sandbox refuses to run as root")
	}
	fake, record := fakeBubblewrap(t)
	t.Setenv("SONGSTEAD_DATA_DIR", privateDir(t))
	var out bytes.Buffer
	if err := runSandbox(context.Background(), []string{"--check", "--bwrap", fake}, &out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(got), "\n--\n/app/server\n--help\n") || strings.Contains(string(got), "/usr/bin/true") {
		t.Fatalf("check did not run the bound server: %s", got)
	}
}

func TestSandboxRejectsExtraMountsAndBadCertificateBundles(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the sandbox refuses to run as root")
	}
	fake, record := fakeBubblewrap(t)
	t.Setenv("SONGSTEAD_DATA_DIR", privateDir(t))
	for _, args := range [][]string{
		{"--check", "--bwrap", fake, "--write-dir", t.TempDir()},
		{"--check", "--bwrap", fake, "--read-file", "/etc/hosts"},
	} {
		if err := runSandbox(context.Background(), args, &bytes.Buffer{}); err == nil {
			t.Errorf("accepted unsupported option %q", args[3])
		}
	}
	for _, bundle := range []string{"relative.pem", "/missing/ca.pem", t.TempDir(), "/tmp/ca\n.pem"} {
		t.Setenv("SSL_CERT_FILE", bundle)
		if err := runSandbox(context.Background(), []string{"--check", "--bwrap", fake}, &bytes.Buffer{}); err == nil {
			t.Errorf("accepted certificate bundle %q", bundle)
		}
	}
	if _, err := os.Stat(record); !os.IsNotExist(err) {
		t.Fatal("invalid settings still launched Bubblewrap")
	}
}

// sandboxCheckArguments runs the sandbox check against a fake Bubblewrap and
// returns the arguments it was given, one per element.
func sandboxCheckArguments(t *testing.T) []string {
	t.Helper()
	fake, record := fakeBubblewrap(t)
	if err := runSandbox(context.Background(), []string{"--check", "--bwrap", fake}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(got), "\n"), "\n")
}

// Songstead never builds sandboxes of its own, so the confined server must not
// be able to create user namespaces either.
func TestSandboxDisablesNestedUserNamespaces(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the sandbox refuses to run as root")
	}
	t.Setenv("SONGSTEAD_DATA_DIR", privateDir(t))
	args := sandboxCheckArguments(t)
	separator := slices.Index(args, "--")
	if separator < 0 || !slices.Contains(args[:separator], "--disable-userns") {
		t.Fatalf("the service policy leaves user namespaces available: %q", args)
	}
	if !slices.Contains(args[:separator], "--unshare-user") {
		t.Fatalf("--disable-userns needs its own user namespace: %q", args)
	}
}

// A private CA bundle named by SSL_CERT_FILE reaches the server read-only at a
// fixed path, and the data directory is the only writable host path.
func TestSandboxBindsTheCustomCertificateBundle(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the sandbox refuses to run as root")
	}
	data := privateDir(t)
	bundle := filepath.Join(t.TempDir(), "private ca.pem")
	if err := os.WriteFile(bundle, []byte("-----BEGIN CERTIFICATE-----\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SONGSTEAD_DATA_DIR", data)
	t.Setenv("SSL_CERT_FILE", bundle)
	args := sandboxCheckArguments(t)
	joined := "\n" + strings.Join(args, "\n") + "\n"
	if !strings.Contains(joined, "\n--ro-bind\n"+bundle+"\n/app/ca-bundle.crt\n") {
		t.Fatalf("custom bundle not bound read-only: %q", args)
	}
	if strings.Count(joined, "\n--bind\n") != 1 || !strings.Contains(joined, "\n--bind\n"+data+"\n"+data+"\n") {
		t.Fatalf("the data directory is not the one writable mount: %q", args)
	}
}

// A setuid Bubblewrap would run the policy with privileges the sandbox is
// meant to do without, so it is refused before anything is launched.
func TestSandboxRejectsSetuidBubblewrap(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the sandbox refuses to run as root")
	}
	fake, record := fakeBubblewrap(t)
	if err := os.Chmod(fake, 0700|os.ModeSetuid); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(fake); err != nil || info.Mode()&os.ModeSetuid == 0 {
		t.Skip("the test directory does not keep the setuid bit")
	}
	t.Setenv("SONGSTEAD_DATA_DIR", privateDir(t))
	if err := runSandbox(context.Background(), []string{"--check", "--bwrap", fake}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "setuid") {
		t.Fatalf("setuid launcher accepted: %v", err)
	}
	if _, err := os.Stat(record); !os.IsNotExist(err) {
		t.Fatal("a setuid launcher was run")
	}
}

func TestSandboxFailureDoesNotCreateDatabase(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("sandbox requires an unprivileged service user")
	}
	data := privateDir(t)
	for _, binary := range []string{"/missing/bwrap", "/usr/bin/false"} {
		if err := run(context.Background(), []string{"sandbox", "--data-dir", data, "--check", "--bwrap", binary}, strings.NewReader(""), &bytes.Buffer{}); err == nil {
			t.Fatal("failed sandbox accepted")
		}
		if _, err := os.Stat(filepath.Join(data, "songstead.db")); !os.IsNotExist(err) {
			t.Fatal("failed sandbox changed data")
		}
	}
}

func TestSandboxRejectsUnsafeDataDirectories(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("sandbox requires an unprivileged service user")
	}
	fake, record := fakeBubblewrap(t)
	broad := t.TempDir()
	if err := os.Chmod(broad, 0755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{broad, "/tmp", "/etc", "/missing/directory"} {
		if err := runSandbox(context.Background(), []string{"--check", "--data-dir", path, "--bwrap", fake}, &bytes.Buffer{}); err == nil {
			t.Fatalf("unsafe directory accepted: %q", path)
		}
	}
	if _, err := os.Stat(record); !os.IsNotExist(err) {
		t.Fatal("invalid data directory launched Bubblewrap")
	}
}

func TestSandboxEnvironmentIsRestricted(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"plain", "value with spaces", "with=equals"} {
		data := privateDir(t)
		args, env, err := servicePolicy(data, executable, []string{"SONGSTEAD_ADDR=" + value, "UNRELATED_SECRET=hidden", "LD_PRELOAD=/tmp/untrusted.so", "SONGSTEAD_DATA_DIR=/unmounted"}).Policy()
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(env, "SONGSTEAD_ADDR="+value) || !slices.Contains(env, "SONGSTEAD_DATA_DIR="+data) {
			t.Fatalf("application configuration not preserved: %q", env)
		}
		for _, entry := range env {
			if strings.HasPrefix(entry, "LD_") || strings.HasPrefix(entry, "UNRELATED_SECRET=") || entry == "SONGSTEAD_DATA_DIR=/unmounted" {
				t.Fatal("unsafe inherited environment")
			}
		}
		if slices.Contains(args, "--unshare-net") {
			t.Fatal("HTTP server cannot use host network")
		}
	}
}
