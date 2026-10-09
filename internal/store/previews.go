// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/airencracken/songstead/internal/media"
)

func (s *Store) PreviewPending(ctx context.Context, viewer, id int64) (bool, error) {
	item, err := s.Recommendation(ctx, viewer, id)
	if err != nil {
		return false, err
	}
	var pending bool
	err = s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM metadata_jobs WHERE media_id=? AND attempts<3)", item.MediaID).Scan(&pending)
	return pending, err
}

func (s *Store) RetryPreview(ctx context.Context, viewer, id int64) error {
	item, err := s.Recommendation(ctx, viewer, id)
	if err != nil {
		return err
	}
	if !media.SupportsMetadata(item.URL) {
		return ErrInvalid
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO metadata_jobs(media_id) VALUES(?) ON CONFLICT(media_id) DO UPDATE SET attempts=0,next_attempt=0", item.MediaID)
	return err
}

// Label choices use the same audience predicate as recommendations. Private
// labels never appear in Recent's picker, including for their own sender.
func (s *Store) LabelChoices(ctx context.Context, viewer int64, recent bool) ([]string, []string, error) {
	where := visible
	if recent {
		where += " AND r.visibility='members' AND r.group_id IS NULL"
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT l.genre,l.tags FROM recommendation_labels l JOIN recommendations r ON r.id=l.recommendation_id JOIN recommendation_destinations d ON d.recommendation_id=r.id WHERE `+where, viewer, viewer, viewer)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	genres, tags := map[string]string{}, map[string]string{}
	for rows.Next() {
		var genre, raw string
		if err = rows.Scan(&genre, &raw); err != nil {
			return nil, nil, err
		}
		if genre != "" {
			genres[strings.ToLower(genre)] = genre
		}
		var values []string
		if err = json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, nil, err
		}
		for _, tag := range values {
			tags[strings.ToLower(tag)] = tag
		}
	}
	return sortedChoices(genres), sortedChoices(tags), rows.Err()
}

func sortedChoices(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}
