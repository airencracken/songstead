// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"errors"
	"github.com/airencracken/comfylib/sandbox"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// killTest stops a test process that may already have exited.
func killTest(t testing.TB, process *os.Process) {
	t.Helper()
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Error(err)
	}
}

func TestRealSandboxedServerLifecycle(t *testing.T) {
	if os.Getenv("COMFYWARE_SANDBOX_TEST") != "1" {
		t.Skip("set COMFYWARE_SANDBOX_TEST=1 to require native sandbox integration")
	}
	root := t.TempDir()
	binary := filepath.Join(root, "songstead")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", binary, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %s %v", out, err)
	}
	data := filepath.Join(root, "data with spaces")
	if err := os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	environment := append(os.Environ(), "SONGSTEAD_DATA_DIR="+data, "SONGSTEAD_ADDR="+address)

	check := exec.Command(binary, "sandbox", "--check")
	check.Env = environment
	if out, err := check.CombinedOutput(); err != nil || !strings.Contains(string(out), "sandbox is ready") {
		t.Fatalf("check: %s %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(data, "songstead.db")); !os.IsNotExist(err) {
		t.Fatal("check created a database")
	}
	failed := exec.Command(binary, "sandbox", "--bwrap", "/usr/bin/false")
	failed.Env = environment
	if out, err := failed.CombinedOutput(); err == nil {
		t.Fatalf("sandbox failure started server: %s", out)
	}
	if _, err := os.Stat(filepath.Join(data, "songstead.db")); !os.IsNotExist(err) {
		t.Fatal("failed sandbox created a database")
	}
	command := exec.Command(binary, "sandbox")
	command.Env = environment
	var log bytes.Buffer
	command.Stdout, command.Stderr = &log, &log
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { killTest(t, command.Process) })
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	ready := false
	client := &http.Client{Timeout: time.Second}
	deadline := time.After(10 * time.Second)
	for !ready {
		response, err := client.Get("http://" + address + "/healthz")
		if err == nil {
			ready = response.StatusCode == 200
			if err := response.Body.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if ready {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("server stopped: %v\n%s", err, &log)
		case <-deadline:
			killTest(t, command.Process)
			<-done
			t.Fatalf("server did not start:\n%s", &log)
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("graceful shutdown: %v\n%s", err, &log)
		}
	case <-time.After(5 * time.Second):
		killTest(t, command.Process)
		<-done
		t.Fatalf("shutdown timed out:\n%s", &log)
	}
	if info, err := os.Stat(filepath.Join(data, "songstead.db")); err != nil || info.Size() == 0 {
		t.Fatalf("no persistent database: %v", err)
	}
}

func TestRealSandboxFilesystemBoundaries(t *testing.T) {
	if os.Getenv("COMFYWARE_SANDBOX_TEST") != "1" {
		t.Skip("set COMFYWARE_SANDBOX_TEST=1 to require namespace tests")
	}
	data := privateDir(t)
	hidden := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(hidden, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	mounts, env, err := servicePolicy(data, executable, []string{"UNRELATED_SECRET=hidden"}).Policy()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := sandbox.Binary("bwrap")
	if err != nil {
		t.Fatal(err)
	}
	script := `test ! -e "$1" && test ! -e /home && test ! -e /root && test -z "${UNRELATED_SECRET:-}" && touch "$2/allowed" && touch /tmp/private-temporary && ! touch /etc/hosts`
	if err := sandbox.Check(context.Background(), binary, mounts, env, "/bin/sh", "-c", script, "probe", hidden, data); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(data, "allowed")); err != nil {
		t.Fatal("data directory is not writable")
	}
	if _, err := os.Stat("/tmp/private-temporary"); !os.IsNotExist(err) {
		t.Fatal("sandbox temporary file escaped onto host")
	}
}
