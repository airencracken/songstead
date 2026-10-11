// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log/slog"
	"os"

	"github.com/airencracken/comfylib/autobackup"
	"github.com/airencracken/songstead/internal/store"
)

func automaticBackups(s *store.Store, dataDir string) (*autobackup.Runner, error) {
	cfg, err := autobackup.Load("songstead", dataDir, os.Getenv)
	if err != nil {
		return nil, err
	}
	return autobackup.New(cfg, s.AutomaticSnapshot, slog.Default())
}
