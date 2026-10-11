// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"os"
	"path/filepath"

	"github.com/airencracken/comfylib/autobackup"
)

// AutomaticSnapshot includes the whole authoritative database. Artwork is a
// refetchable cache; credentials, preferences, avatars and private music live
// in the database and pass the existing restore validator.
func (s *Store) AutomaticSnapshot(ctx context.Context, destination string, limit int64) error {
	if err := autobackup.CheckDatabase(ctx, s.db, limit); err != nil {
		return err
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		return err
	}
	path := filepath.Join(destination, "songstead.db")
	if err := s.Backup(ctx, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	return ValidateSnapshot(ctx, path)
}
