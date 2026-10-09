// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/airencracken/songstead/internal/annotations"
	"github.com/airencracken/songstead/internal/media"
	"strings"
	"time"
)

type Recording struct {
	ID                 int64
	URL, Title         string
	Duration, Position int
}
type Offset struct {
	RecordingID         int64
	Seconds, Start, End int
}

func recording(ctx context.Context, tx *sql.Tx, mid int64, key, raw, title string, duration int) (int64, error) {
	_, err := tx.ExecContext(ctx, "INSERT INTO recordings(canonical_key,url,title,duration) VALUES(?,?,?,?) ON CONFLICT(canonical_key) DO NOTHING", key, raw, title, duration)
	if err != nil {
		return 0, err
	}
	var id int64
	if err = tx.QueryRowContext(ctx, "SELECT id FROM recordings WHERE canonical_key=?", key).Scan(&id); err != nil {
		return 0, err
	}
	if duration > 0 {
		var latest int
		if err = tx.QueryRowContext(ctx, "SELECT coalesce(max(seconds),0) FROM annotation_offsets WHERE recording_id=?", id).Scan(&latest); err != nil {
			return 0, err
		}
		if latest > duration {
			return 0, ErrInvalid
		}
		if _, err = tx.ExecContext(ctx, "UPDATE recordings SET duration=? WHERE id=?", duration, id); err != nil {
			return 0, err
		}
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO media_recordings VALUES(?,?) ON CONFLICT DO NOTHING", mid, id)
	return id, err
}
func (s *Store) AddRecording(ctx context.Context, viewer, id int64, raw, title string, duration int) error {
	if !validText(title, 1, 160) || duration < 0 || duration > 86400 {
		return ErrInvalid
	}
	key, kind, err := media.Identity(raw)
	if err != nil {
		return ErrInvalid
	}
	if kind == "album" || kind == "artist" {
		return ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var mid int64
	var musicKind string
	err = tx.QueryRowContext(ctx, `SELECT r.media_id,m.kind FROM recommendations r JOIN media m ON m.id=r.media_id JOIN recommendation_destinations d ON d.recommendation_id=r.id WHERE r.id=? AND `+visible, id, viewer, viewer, viewer).Scan(&mid, &musicKind)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMissing
	}
	if err != nil {
		return err
	}
	if musicKind == "artist" {
		return ErrInvalid
	}
	// A track has one identity; album tracks are explicitly named and selected.
	if musicKind == "track" {
		var identity string
		if err = tx.QueryRowContext(ctx, "SELECT canonical_key FROM media WHERE id=?", mid).Scan(&identity); err != nil {
			return err
		}
		if key != identity {
			return ErrInvalid
		}
	}
	if _, err = recording(ctx, tx, mid, key, raw, title, duration); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Recordings(ctx context.Context, viewer, id int64) ([]Recording, error) {
	r, err := s.Recommendation(ctx, viewer, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT t.id,t.url,t.title,t.duration,coalesce(p.seconds,-1) FROM recordings t JOIN media_recordings m ON m.recording_id=t.id LEFT JOIN listening_positions p ON p.recording_id=t.id AND p.user_id=? WHERE m.media_id=? ORDER BY t.id`, viewer, r.MediaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recording{}
	for rows.Next() {
		var t Recording
		if err = rows.Scan(&t.ID, &t.URL, &t.Title, &t.Duration, &t.Position); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (s *Store) Annotate(ctx context.Context, viewer, id, track int64, body string) error {
	if !validText(body, 1, 2000) || strings.TrimSpace(body) == "" {
		return ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var mid int64
	var kind string
	err = tx.QueryRowContext(ctx, `SELECT r.media_id,m.kind FROM recommendations r JOIN media m ON m.id=r.media_id JOIN recommendation_destinations d ON d.recommendation_id=r.id WHERE r.id=? AND `+visible, id, viewer, viewer, viewer).Scan(&mid, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMissing
	}
	if err != nil {
		return err
	}
	settings, err := readSettings(ctx, tx, DefaultSettings("", ""))
	if err != nil {
		return err
	}
	if !settings.LocalComments() {
		return ErrForbidden
	}
	duration := 0
	if track == 0 && kind == "track" {
		err = tx.QueryRowContext(ctx, `SELECT recording_id FROM media_recordings WHERE media_id=?`, mid).Scan(&track)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if track != 0 {
		err = tx.QueryRowContext(ctx, `SELECT t.duration FROM recordings t JOIN media_recordings m ON m.recording_id=t.id WHERE t.id=? AND m.media_id=?`, track, mid).Scan(&duration)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalid
		}
		if err != nil {
			return err
		}
	}
	refs := annotations.Parse(body, duration)
	if track == 0 && len(refs) > 0 {
		return ErrInvalid
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO comments(recommendation_id,author_id,body,created_at) VALUES(?,?,?,?)", id, viewer, body, time.Now().Unix())
	if err != nil {
		return err
	}
	cid, err := result.LastInsertId()
	if err != nil {
		return err
	}
	for i, ref := range refs {
		if _, err = tx.ExecContext(ctx, "INSERT INTO annotation_offsets VALUES(?,?,?,?,?,?)", cid, track, i, ref.Seconds, ref.Start, ref.End); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) comments(ctx context.Context, viewer, id int64) ([]Comment, error) {
	if _, err := s.Recommendation(ctx, viewer, id); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.author_id,c.created_at,u.username,c.body FROM comments c JOIN users u ON u.id=c.author_id JOIN recommendations r ON r.id=c.recommendation_id JOIN recommendation_destinations d ON d.recommendation_id=r.id WHERE c.recommendation_id=? AND `+visible+` ORDER BY c.id`, id, viewer, viewer, viewer)
	if err != nil {
		return nil, err
	}
	list := []Comment{}
	for rows.Next() {
		var c Comment
		if err = rows.Scan(&c.ID, &c.AuthorID, &c.CreatedAt, &c.Author, &c.Body); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range list {
		rows, err := s.db.QueryContext(ctx, "SELECT recording_id,seconds,start_byte,end_byte FROM annotation_offsets WHERE comment_id=? ORDER BY ordinal", list[i].ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var o Offset
			if err = rows.Scan(&o.RecordingID, &o.Seconds, &o.Start, &o.End); err != nil {
				rows.Close()
				return nil, err
			}
			list[i].Offsets = append(list[i].Offsets, o)
			list[i].RecordingID = o.RecordingID
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return list, nil
}
func (s *Store) AnnotationMode(ctx context.Context, user int64) (string, error) {
	var mode string
	err := s.db.QueryRowContext(ctx, "SELECT annotation_mode FROM users WHERE id=?", user).Scan(&mode)
	return mode, err
}
func (s *Store) SetAnnotationMode(ctx context.Context, user int64, mode string) error {
	if mode != "immediate" && mode != "spoiler-free" && mode != "hidden" {
		return ErrInvalid
	}
	res, err := s.db.ExecContext(ctx, "UPDATE users SET annotation_mode=? WHERE id=?", mode, user)
	return mutationResult(res, err)
}
func (s *Store) SetPosition(ctx context.Context, viewer, id, track int64, seconds int) error {
	if seconds < 0 || seconds > 86400 {
		return ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var mid int64
	err = tx.QueryRowContext(ctx, `SELECT r.media_id FROM recommendations r JOIN recommendation_destinations d ON d.recommendation_id=r.id WHERE r.id=? AND `+visible, id, viewer, viewer, viewer).Scan(&mid)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMissing
	}
	if err != nil {
		return err
	}
	var duration int
	err = tx.QueryRowContext(ctx, `SELECT t.duration FROM recordings t JOIN media_recordings m ON m.recording_id=t.id WHERE t.id=? AND m.media_id=?`, track, mid).Scan(&duration)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalid
	}
	if err != nil {
		return err
	}
	if duration > 0 && seconds > duration {
		return ErrInvalid
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO listening_positions VALUES(?,?,?) ON CONFLICT(user_id,recording_id) DO UPDATE SET seconds=excluded.seconds", viewer, track, seconds)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// VisibleComments filters before rendering; concealed prose never enters HTML.
// With multiple offsets the entire comment waits for its latest position.
func VisibleComments(comments []Comment, tracks []Recording, mode string, reveal bool) []Comment {
	out := []Comment{}
	positions := map[int64]int{}
	for _, t := range tracks {
		positions[t.ID] = t.Position
	}
	for _, c := range comments {
		if len(c.Offsets) == 0 {
			out = append(out, c)
			continue
		}
		visible := mode == "immediate" || reveal
		if mode == "spoiler-free" && !reveal {
			visible = true
			for _, o := range c.Offsets {
				pos, ok := positions[o.RecordingID]
				if !ok || pos < o.Seconds {
					visible = false
				}
			}
		}
		if visible {
			out = append(out, c)
		}
	}
	return out
}
