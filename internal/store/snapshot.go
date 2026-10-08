// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
)

func ValidateSnapshot(ctx context.Context, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	u := url.URL{Scheme: "file", Path: abs, RawQuery: "mode=ro&_pragma=foreign_keys(1)"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return err
	}
	defer db.Close()
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	// Previous releases used schema 3. Restore it unchanged; the server applies
	// the owner migration on its next start.
	if version != 3 && version != 4 {
		return errors.New("snapshot schema does not match this binary")
	}
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return errors.New("snapshot integrity check failed")
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("snapshot has broken foreign keys")
	}
	if err := rows.Err(); err != nil {
		return err
	}
	// Exercise the actual domain joins so a forged version on an unrelated
	// database cannot be accepted as a Songstead backup.
	rows.Close()
	rows, err = db.QueryContext(ctx, selectRecommendation+" LIMIT 0", 0, 0)
	if err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, query := range []string{
		"SELECT canonical_key,kind FROM media LIMIT 0",
		"SELECT annotation_mode FROM users LIMIT 0",
		"SELECT media_id,user_id,listening,updated_at FROM music_states LIMIT 0",
		"SELECT comment_id,recording_id,ordinal,seconds,start_byte,end_byte FROM annotation_offsets LIMIT 0",
		"SELECT user_id,recording_id,seconds FROM listening_positions LIMIT 0",
		"SELECT media_id,user_id,url FROM discussions LIMIT 0",
		"SELECT id,owner_id,name FROM groups LIMIT 0",
		"SELECT group_id,user_id FROM group_members LIMIT 0",
		"SELECT id,canonical_key,url,title,duration FROM recordings LIMIT 0",
		"SELECT media_id,recording_id FROM media_recordings LIMIT 0",
		"SELECT token_hash,user_id,expires_at FROM sessions LIMIT 0",
		"SELECT id,recommendation_id,author_id,body,created_at FROM comments LIMIT 0",
		"SELECT media_id,attempts,next_attempt FROM metadata_jobs LIMIT 0",
		"SELECT id,username,password_hash,created_at FROM users LIMIT 0",
	} {
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	if version == 4 {
		var invalid int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE role IS NULL OR role NOT IN ('member','owner')").Scan(&invalid); err != nil {
			return err
		}
		if invalid != 0 {
			return errors.New("snapshot has invalid account roles")
		}
	}
	return nil
}
