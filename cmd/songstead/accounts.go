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
	"setup":        "Print the private key for the first-owner setup page.",
	"sandbox":      "Start the server inside Bubblewrap, or test the confinement with --check.",
	"serve":        "Start the server. Settings come from SONGSTEAD_* environment variables.",
	"create-owner": "Create a new owner. Existing accounts are never changed.",
	"create-user":  "Create a new member. Existing accounts are never changed.",
	"set-password": "Set a new password and sign the account out everywhere.",
	"set-role":     "Change an account's role. At least one active owner must remain.",
	"list-users":   "List account IDs, usernames and roles.",
	"backup":       "Save a snapshot of the database to a new file.",
	"restore":      "Restore a snapshot into a new, empty data directory.",
}

var provisioningCommands = map[string]bool{
	"setup":        true,
	"create-owner": true, "create-user": true, "set-password": true,
	"set-role": true, "list-users": true, "backup": true,
}

var geteuid = os.Geteuid

func servicePaths() svcconfig.Paths {
	return svcconfig.Detect("songstead", "/var/lib/songstead")
}

func provisioningRequest(args []string, paths svcconfig.Paths) privdrop.Request {
	return privdrop.Request{Args: args, Commands: provisioningCommands, Paths: paths, DefaultUser: "songstead", Settings: func(command, dataDir string) (map[string]string, error) {
		if command == "setup" {
			return paths.Settings("SONGSTEAD_BASE_URL", "SONGSTEAD_ADDR")
		}
		return nil, nil
	}}
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
