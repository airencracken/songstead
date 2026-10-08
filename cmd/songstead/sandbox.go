// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/airencracken/comfylib/sandbox"
)

func runSandbox(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("sandbox", flag.ContinueOnError)
	flags.SetOutput(out)
	check := flags.Bool("check", false, "verify the sandbox without starting the server")
	dataDir := flags.String("data-dir", envDefault("SONGSTEAD_DATA_DIR", "./data"), "existing private data directory")
	bwrap := flags.String("bwrap", envDefault("SONGSTEAD_BWRAP", "bwrap"), "Bubblewrap executable")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected sandbox arguments; use songstead sandbox --help")
	}
	if os.Geteuid() == 0 {
		return errors.New("run the sandbox as the unprivileged songstead service user")
	}
	data, err := filepath.Abs(*dataDir)
	if err != nil {
		return err
	}
	info, err := os.Stat(data)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("sandbox data directory must exist and be private: chmod 700 it before continuing")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	// Songstead builds no sandboxes of its own, so the server gets no user
	// namespaces: NestedSandbox stays false and the policy adds
	// --disable-userns. SSL_CERT_FILE in the environment names a custom CA
	// bundle, bound read-only at /app/ca-bundle.crt.
	policy := servicePolicy(data, executable, os.Environ())
	mounts, environment, err := policy.Policy()
	if err != nil {
		return err
	}
	binary, err := sandbox.Binary(*bwrap)
	if err != nil {
		return fmt.Errorf("sandbox mode requires Bubblewrap: %w", err)
	}
	// Running the bound server proves the namespaces, mounts and loader work
	// without depending on any particular host utility.
	if err := sandbox.Check(ctx, binary, mounts, environment, "/app/server", "--help"); err != nil {
		return err
	}
	if *check {
		_, err := fmt.Fprintln(out, "Bubblewrap sandbox is ready.")
		return err
	}
	return sandbox.Run(ctx, binary, append(mounts, "--", "/app/server"), environment)
}

func servicePolicy(data, executable string, environment []string) sandbox.Service {
	return sandbox.Service{Prefix: "SONGSTEAD_", DataDir: data, Executable: executable, Env: environment}
}
