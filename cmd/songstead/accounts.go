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
	"text/tabwriter"

	"github.com/airencracken/comfylib/privdrop"
	"github.com/airencracken/comfylib/svcconfig"
	"github.com/airencracken/songstead/internal/store"
)

var commandDescriptions = map[string]string{
	"sandbox":      "Start the server confined by Bubblewrap, or verify isolation with --check.",
	"serve":        "Start the HTTP server. Configuration uses SONGSTEAD_* environment variables.",
	"create-owner": "Create a new owner locally. Existing accounts are never promoted or changed.",
	"create-user":  "Create a new member locally. Existing accounts are never changed.",
	"set-password": "Replace an account's password and revoke all its sessions.",
	"set-role":     "Explicitly change an existing account role; keep at least one active owner.",
	"list-users":   "List local account IDs, usernames, and roles. Password hashes are never shown.",
	"backup":       "Save a consistent SQLite snapshot to a new file.",
	"restore":      "Restore a snapshot into a new, empty data directory.",
}

var provisioningCommands = map[string]bool{
	"create-owner": true, "create-user": true, "set-password": true,
	"set-role": true, "list-users": true, "backup": true,
}

var geteuid = os.Geteuid

func servicePaths() svcconfig.Paths {
	return svcconfig.Detect("songstead", "/var/lib/songstead")
}

func provisioningRequest(args []string, paths svcconfig.Paths) privdrop.Request {
	return privdrop.Request{Args: args, Commands: provisioningCommands, Paths: paths, DefaultUser: "songstead"}
}

func commandFlags(command string, out io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet("songstead "+command, flag.ContinueOnError)
	flags.SetOutput(out)
	flags.Usage = func() {
		_, _ = fmt.Fprintf(out, "Usage: songstead %s [OPTIONS]\n\n%s\n\n", command, commandDescriptions[command])
		flags.PrintDefaults()
	}
	return flags
}

func resolveDataDir(flags *flag.FlagSet, data *string, paths svcconfig.Paths, managed bool) error {
	explicit := false
	flags.Visit(func(f *flag.Flag) { explicit = explicit || f.Name == "data-dir" })
	if explicit && *data == "" {
		return errors.New("--data-dir must not be empty")
	}
	var err error
	if explicit {
		*data, err = filepath.Abs(*data)
	} else if managed {
		*data, err = paths.DataDir("SONGSTEAD_DATA_DIR")
	} else {
		*data, err = filepath.Abs(envDefault("SONGSTEAD_DATA_DIR", "./data"))
	}
	return err
}

func printUsers(ctx context.Context, s *store.Store, out io.Writer) error {
	users, err := s.Accounts(ctx)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "ID\tUSERNAME\tROLE\tSUSPENDED"); err != nil {
		return err
	}
	for _, user := range users {
		if _, err := fmt.Fprintf(w, "%d\t%s\t%s\t%t\n", user.ID, user.Username, user.Role, user.Suspended); err != nil {
			return err
		}
	}
	return w.Flush()
}
