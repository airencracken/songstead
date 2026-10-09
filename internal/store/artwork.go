// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/airencracken/songstead/internal/media"
)

// Upgrade existing links as well as new shares. The transaction includes the
// queue, so a failed migration never leaves a partially upgraded instance.
func queueArtwork(tx *sql.Tx) error {
	rows, err := tx.Query("SELECT id,original_url FROM media")
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		var raw string
		if err = rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		if media.SupportsMetadata(raw) {
			ids = append(ids, id)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = tx.Exec("INSERT INTO metadata_jobs(media_id) VALUES(?) ON CONFLICT(media_id) DO UPDATE SET attempts=0,next_attempt=0", id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Artwork(ctx context.Context, viewer, id int64) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT art.content FROM recommendations r JOIN recommendation_destinations d ON d.recommendation_id=r.id JOIN media_artwork art ON art.media_id=r.media_id WHERE r.id=? AND `+visible, id, viewer, viewer, viewer).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrMissing
	}
	return data, err
}
