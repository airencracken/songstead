// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
)

// Labels describe one recommendation, never the global provider identity. This
// keeps descriptions on private sends inside their original audience.
type Labels struct {
	Genre string
	Tags  []string
}

type DiscoveryPreferences struct {
	ExcludedGenres, PreferredGenres, ExcludedTags, PreferredTags []string
}

func validLabel(value string, max int) bool {
	return validText(value, 1, max) && !strings.Contains(value, ",") && strings.IndexFunc(value, unicode.IsControl) < 0
}

// ParseLabels supports freeform, comma-separated names without introducing a
// fixed genre taxonomy. First spelling wins; comparisons ignore case.
func ParseLabels(raw string, max int) ([]string, error) {
	if len(raw) > 4096 {
		return nil, ErrInvalid
	}
	result := []string{}
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		label := strings.TrimSpace(part)
		if label == "" {
			continue
		}
		if !validLabel(label, max) {
			return nil, ErrInvalid
		}
		key := strings.ToLower(label)
		if !seen[key] {
			result = append(result, label)
			seen[key] = true
		}
	}
	if len(result) > 20 {
		return nil, ErrInvalid
	}
	return result, nil
}

func normalizeLabels(labels Labels) (Labels, error) {
	labels.Genre = strings.TrimSpace(labels.Genre)
	if labels.Genre != "" && !validLabel(labels.Genre, 80) || len(labels.Tags) > 20 {
		return Labels{}, ErrInvalid
	}
	for _, tag := range labels.Tags {
		if !validLabel(strings.TrimSpace(tag), 40) {
			return Labels{}, ErrInvalid
		}
	}
	tags, err := ParseLabels(strings.Join(labels.Tags, ","), 40)
	labels.Tags = tags
	return labels, err
}

func labelJSON(labels []string, lower bool) string {
	copy := make([]string, len(labels))
	for i, label := range labels {
		if lower {
			label = strings.ToLower(label)
		}
		copy[i] = label
	}
	data, _ := json.Marshal(copy)
	return string(data)
}

func saveLabels(ctx context.Context, tx *sql.Tx, id int64, labels Labels) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO recommendation_labels VALUES(?,?,?,?,?) ON CONFLICT(recommendation_id) DO UPDATE SET genre=excluded.genre,genre_key=excluded.genre_key,tags=excluded.tags,tag_keys=excluded.tag_keys`, id, labels.Genre, strings.ToLower(labels.Genre), labelJSON(labels.Tags, false), labelJSON(labels.Tags, true))
	return err
}

func (s *Store) SetLabels(ctx context.Context, actor, id int64, labels Labels) error {
	labels, err := normalizeLabels(labels)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var yes int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM recommendations r JOIN recommendation_destinations d ON d.recommendation_id=r.id WHERE r.id=? AND r.sender_id=? AND `+visible, id, actor, actor, actor, actor).Scan(&yes)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMissing
	}
	if err != nil {
		return err
	}
	if err = saveLabels(ctx, tx, id, labels); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RecommendLabeledMusic(ctx context.Context, sender, recipient, group int64, raw, note, kind, title, artist string, labels Labels) (int64, error) {
	return s.recommend(ctx, sender, recipient, group, raw, note, kind, title, artist, "private", &labels)
}

func (s *Store) ShareLabeledMusic(ctx context.Context, sender int64, raw, note, kind, title, artist string, labels Labels) (int64, error) {
	return s.recommend(ctx, sender, 0, 0, raw, note, kind, title, artist, "members", &labels)
}

func normalizePreferences(p DiscoveryPreferences) (DiscoveryPreferences, error) {
	for _, field := range []struct {
		values *[]string
		max    int
	}{
		{&p.ExcludedGenres, 80}, {&p.PreferredGenres, 80}, {&p.ExcludedTags, 40}, {&p.PreferredTags, 40},
	} {
		if len(*field.values) > 20 {
			return DiscoveryPreferences{}, ErrInvalid
		}
		for _, value := range *field.values {
			if !validLabel(strings.TrimSpace(value), field.max) {
				return DiscoveryPreferences{}, ErrInvalid
			}
		}
		values, err := ParseLabels(strings.Join(*field.values, ","), field.max)
		if err != nil {
			return DiscoveryPreferences{}, err
		}
		*field.values = values
	}
	return p, nil
}

func (s *Store) SetDiscoveryPreferences(ctx context.Context, user int64, p DiscoveryPreferences) error {
	p, err := normalizePreferences(p)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO discovery_preferences VALUES(?,?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET excluded_genres=excluded.excluded_genres,preferred_genres=excluded.preferred_genres,excluded_tags=excluded.excluded_tags,preferred_tags=excluded.preferred_tags`, user, labelJSON(p.ExcludedGenres, false), labelJSON(p.PreferredGenres, false), labelJSON(p.ExcludedTags, false), labelJSON(p.PreferredTags, false))
	return err
}

func readDiscoveryPreferences(ctx context.Context, db settingsReader, user int64) (DiscoveryPreferences, error) {
	var p DiscoveryPreferences
	var raw [4]string
	err := db.QueryRowContext(ctx, "SELECT excluded_genres,preferred_genres,excluded_tags,preferred_tags FROM discovery_preferences WHERE user_id=?", user).Scan(&raw[0], &raw[1], &raw[2], &raw[3])
	if errors.Is(err, sql.ErrNoRows) {
		return normalizePreferences(p)
	}
	if err != nil {
		return p, err
	}
	for i, field := range []*[]string{&p.ExcludedGenres, &p.PreferredGenres, &p.ExcludedTags, &p.PreferredTags} {
		if err := json.Unmarshal([]byte(raw[i]), field); err != nil {
			return p, err
		}
	}
	return normalizePreferences(p)
}

func (s *Store) DiscoveryPreferences(ctx context.Context, user int64) (DiscoveryPreferences, error) {
	return readDiscoveryPreferences(ctx, s.db, user)
}

// Bound JSON parameters keep matching literal, including SQL punctuation and
// wildcard characters. Exclusions are applied before preference ranking.
const preferredMatch = `(coalesce(l.genre_key,'') IN (SELECT value FROM json_each(?)) OR EXISTS(SELECT 1 FROM json_each(coalesce(l.tag_keys,'[]')) tags WHERE tags.value IN (SELECT value FROM json_each(?))))`
