// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"database/sql/driver"
	"strings"
	"unicode"
	"unicode/utf8"

	"modernc.org/sqlite"
)

// SQLite's built-in lower only folds ASCII. Use Go's Unicode mapping for names,
// tags and comments, without treating user input as SQL or wildcard syntax.
func init() {
	sqlite.MustRegisterDeterministicScalarFunction("songstead_fold", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		value, ok := args[0].(string)
		if !ok {
			return "", nil
		}
		return strings.ToLower(value), nil
	})
}

func SearchQuery(raw string) (string, error) {
	if !utf8.ValidString(raw) || utf8.RuneCountInString(raw) > 200 {
		return "", ErrInvalid
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return "", ErrInvalid
		}
	}
	return strings.TrimSpace(raw), nil
}

func searchBounds(viewer, before int64, limit int, raw string) (string, error) {
	query, err := SearchQuery(raw)
	if err != nil || viewer <= 0 || before < 0 || limit < 1 || limit > 100 {
		return "", ErrInvalid
	}
	return strings.ToLower(query), nil
}

// SearchMusic includes only recommendations visible to an active viewer.
// Personal listening notes and ratings are never searched. IDs keep older pages
// stable if another recommendation arrives between requests.
func (s *Store) SearchMusic(ctx context.Context, viewer int64, raw string, before int64, limit int) ([]Recommendation, error) {
	query, err := searchBounds(viewer, before, limit, raw)
	if err != nil {
		return nil, err
	}
	out := []Recommendation{}
	if query == "" {
		return out, nil
	}
	candidate := ""
	params := []any{viewer, viewer, viewer, viewer, viewer, viewer}
	if utf8.RuneCountInString(query) >= 3 {
		// Bound candidate collection. Common phrases use the ID-ordered scan so
		// finding the first page never sorts the entire matching collection.
		candidates, e := s.db.QueryContext(ctx, `SELECT rowid FROM music_search WHERE music_search MATCH ? LIMIT 257`, `"`+strings.ReplaceAll(query, `"`, `""`)+`"`)
		if e != nil {
			return nil, e
		}
		ids := []int64{}
		for candidates.Next() {
			var id int64
			if e = candidates.Scan(&id); e != nil {
				candidates.Close()
				return nil, e
			}
			ids = append(ids, id)
		}
		e = candidates.Err()
		candidates.Close()
		if e != nil {
			return nil, e
		}
		if len(ids) == 0 {
			return out, nil
		}
		if len(ids) <= 256 {
			candidate = " AND r.id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + ")"
			for _, id := range ids {
				params = append(params, id)
			}
		}
	}
	params = append(params, before, before, query, query, query, query, query, limit)
	rows, err := s.db.QueryContext(ctx, selectRecommendation+` WHERE `+visible+`
 AND EXISTS(SELECT 1 FROM users v WHERE v.id=? AND v.suspended=0)`+candidate+`
 AND (?=0 OR r.id<?)
 AND (instr(songstead_fold(coalesce(r.title_override,m.title)),?)>0 OR instr(songstead_fold(coalesce(r.artist_override,m.artist)),?)>0
 OR instr(songstead_fold(r.note),?)>0 OR instr(songstead_fold(coalesce(l.genre,'')),?)>0
 OR EXISTS(SELECT 1 FROM json_each(coalesce(l.tags,'[]')) t WHERE instr(songstead_fold(t.value),?)>0))
 ORDER BY r.id DESC LIMIT ?`, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanRecommendation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// SearchComments applies audience and spoiler rules before limiting results.
// It includes authorized private conversations, unlike the community feed.
func (s *Store) SearchComments(ctx context.Context, viewer int64, raw string, before int64, limit int) ([]RecentComment, error) {
	query, err := searchBounds(viewer, before, limit, raw)
	if err != nil {
		return nil, err
	}
	out := []RecentComment{}
	if query == "" {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.author_id,c.created_at,u.username,c.body,r.id,coalesce(r.title_override,m.title),coalesce(r.artist_override,m.artist),
 EXISTS(SELECT 1 FROM media_artwork a WHERE a.media_id=m.id)
 FROM comments c JOIN recommendations r ON r.id=c.recommendation_id
 JOIN recommendation_destinations d ON d.recommendation_id=r.id JOIN media m ON m.id=r.media_id
 JOIN users u ON u.id=c.author_id JOIN users v ON v.id=? AND v.suspended=0
 WHERE `+visible+` AND `+commentVisible+`
 AND (?=0 OR c.id<?) AND instr(songstead_fold(c.body),?)>0 ORDER BY c.id DESC LIMIT ?`, viewer, viewer, viewer, viewer, before, before, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c RecentComment
		if err := rows.Scan(&c.ID, &c.AuthorID, &c.CreatedAt, &c.Author, &c.Body, &c.RecommendationID, &c.Title, &c.Artist, &c.HasArtwork); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
