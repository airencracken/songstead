// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"errors"
	"unicode/utf8"

	"github.com/airencracken/songstead/internal/media"
)

type PostEdit struct {
	URL, Note, Kind, Title, Artist string
	Labels                         Labels
}

// Author checks run inside the write transaction, including current membership
// and suspension, so a visible post alone never grants mutation rights.
func ownPost(ctx context.Context, tx *sql.Tx, actor, id int64) (int64, error) {
	var mid int64
	err := tx.QueryRowContext(ctx, `SELECT r.media_id FROM recommendations r JOIN recommendation_destinations d ON d.recommendation_id=r.id WHERE r.id=? AND r.sender_id=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND suspended=0) AND `+visible, id, actor, actor, actor, actor, actor).Scan(&mid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrMissing
	}
	return mid, err
}

// EditPost leaves the audience, author, creation time and other posts unchanged.
// Music-level likes and listening history stay with their original recording.
func (s *Store) EditPost(ctx context.Context, actor, id int64, edit PostEdit) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	old, err := ownPost(ctx, tx, actor, id)
	if err != nil {
		return err
	}
	link, err := media.Parse(edit.URL)
	if err != nil {
		return ErrInvalid
	}
	key, inferred, err := media.Identity(edit.URL)
	if err != nil {
		return ErrInvalid
	}
	if edit.Kind == "" {
		edit.Kind = inferred
	}
	if edit.Kind != "link" && edit.Kind != "track" && edit.Kind != "album" && edit.Kind != "artist" || !validText(edit.Title, 0, 160) || !validText(edit.Artist, 0, 160) || !validText(edit.Note, 0, 2000) {
		return ErrInvalid
	}
	labels, err := normalizeLabels(edit.Labels)
	if err != nil {
		return err
	}
	var title any
	if edit.Title != "" {
		title = edit.Title
	}
	fallback := link.Title
	if utf8.RuneCountInString(fallback) > 160 {
		fallback = link.Provider + " link"
	}
	// Never revise shared provider metadata while correcting one post.
	_, err = tx.ExecContext(ctx, `INSERT INTO media(original_url,provider,title,media_type,video_id,canonical_key,kind) VALUES(?,?,?,?,?,?,?) ON CONFLICT(canonical_key) DO NOTHING`, link.Original, link.Provider, fallback, link.Type, link.VideoID, key, inferred)
	if err != nil {
		return err
	}
	var mid int64
	if err = tx.QueryRowContext(ctx, "SELECT id FROM media WHERE canonical_key=?", key).Scan(&mid); err != nil {
		return err
	}
	if mid != old {
		// Keep existing timestamp references attached to their original music.
		var annotated bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM annotation_offsets o JOIN comments c ON c.id=o.comment_id WHERE c.recommendation_id=?)`, id).Scan(&annotated); err != nil {
			return err
		}
		if annotated {
			return ErrInvalid
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE recommendations SET media_id=?,source_url=?,note=?,title_override=?,artist_override=?,kind_override=? WHERE id=?`, mid, edit.URL, edit.Note, title, edit.Artist, edit.Kind, id); err != nil {
		return err
	}
	if err = saveLabels(ctx, tx, id, labels); err != nil {
		return err
	}
	if edit.Kind == "track" {
		recordingTitle := edit.Title
		if recordingTitle == "" {
			recordingTitle = fallback
		}
		if _, err = recording(ctx, tx, mid, key, edit.URL, recordingTitle, 0); err != nil {
			return err
		}
	}
	if media.SupportsMetadata(edit.URL) {
		if _, err = tx.ExecContext(ctx, `INSERT INTO metadata_jobs(media_id) SELECT ? WHERE NOT EXISTS(SELECT 1 FROM media_artwork WHERE media_id=?) ON CONFLICT(media_id) DO UPDATE SET attempts=0,next_attempt=0`, mid, mid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeletePost cascades only post-owned labels, comments and reactions. Shared
// music, other recommendations and each listener's private music state survive.
func (s *Store) DeletePost(ctx context.Context, actor, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = ownPost(ctx, tx, actor, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM recommendations WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}
