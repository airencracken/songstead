// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/airencracken/comfylib/reference"
	"github.com/airencracken/songstead/internal/annotations"
	"github.com/airencracken/songstead/internal/media"
)

type Group struct {
	Members     map[int64]bool
	ID, OwnerID int64
	Name        string
}
type Filter struct {
	Recent         bool
	GroupByMusic   bool
	Status, Kind   string
	Genre, Tag     string
	UsePreferences bool
	Person, Group  int64
}

func migrateMusic(tx *sql.Tx) error {
	rows, err := tx.Query("SELECT id,original_url FROM media ORDER BY id")
	if err != nil {
		return err
	}
	type item struct {
		id  int64
		raw string
	}
	var items []item
	for rows.Next() {
		var i item
		if err = rows.Scan(&i.id, &i.raw); err != nil {
			rows.Close()
			return err
		}
		items = append(items, i)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	seen := map[string]int64{}
	for _, i := range items {
		key, kind, err := media.Identity(i.raw)
		if err != nil {
			return err
		}
		if first, ok := seen[key]; ok {
			if _, err = tx.Exec("UPDATE recommendations SET media_id=? WHERE media_id=?", first, i.id); err != nil {
				return err
			}
			if _, err = tx.Exec("DELETE FROM media WHERE id=?", i.id); err != nil {
				return err
			}
		} else {
			seen[key] = i.id
			if _, err = tx.Exec("UPDATE media SET canonical_key=?,kind=? WHERE id=?", key, kind, i.id); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec("CREATE UNIQUE INDEX media_identity ON media(canonical_key)"); err != nil {
		return err
	}
	// Latest explicit choice wins when old duplicate recommendations disagree.
	_, err = tx.Exec(`INSERT INTO music_states SELECT r.media_id,x.user_id,x.listening,x.updated_at FROM reactions x JOIN recommendations r ON r.id=x.recommendation_id WHERE x.recommendation_id=(SELECT x2.recommendation_id FROM reactions x2 JOIN recommendations r2 ON r2.id=x2.recommendation_id WHERE r2.media_id=r.media_id AND x2.user_id=x.user_id ORDER BY x2.updated_at DESC,x2.recommendation_id DESC LIMIT 1)`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO recordings(canonical_key,url,title) SELECT canonical_key,original_url,substr(title,1,160) FROM media WHERE kind='track'`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO media_recordings SELECT m.id,t.id FROM media m JOIN recordings t ON t.canonical_key=m.canonical_key`)
	if err != nil {
		return err
	}
	rows, err = tx.Query(`SELECT c.id,c.body,mr.recording_id FROM comments c JOIN recommendations r ON r.id=c.recommendation_id JOIN media m ON m.id=r.media_id JOIN media_recordings mr ON mr.media_id=m.id WHERE m.kind='track'`)
	if err != nil {
		return err
	}
	type oldComment struct {
		id, track int64
		body      string
	}
	var old []oldComment
	for rows.Next() {
		var c oldComment
		if err = rows.Scan(&c.id, &c.body, &c.track); err != nil {
			rows.Close()
			return err
		}
		old = append(old, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, c := range old {
		for i, ref := range annotations.Parse(c.body, 0) {
			if _, err = tx.Exec("INSERT INTO annotation_offsets VALUES(?,?,?,?,?,?)", c.id, c.track, i, ref.Seconds, ref.Start, ref.End); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) recommend(ctx context.Context, sender, recipient, group int64, raw, note, kind, title, artist, visibility string, labels *Labels) (int64, error) {
	if visibility != "private" && visibility != "members" || group < 0 || visibility == "members" && (group != 0 || recipient != 0) || visibility == "private" && group == 0 && (recipient <= 0 || sender == recipient) {
		return 0, ErrInvalid
	}
	if visibility == "members" {
		// The destination row remains singular; sharing never fans out individual gifts.
		recipient = sender
	}
	link, err := media.Parse(raw)
	if err != nil {
		return 0, err
	}
	key, inferred, err := media.Identity(raw)
	if err != nil {
		return 0, err
	}
	if kind == "" {
		kind = inferred
	}
	if kind != "track" && kind != "album" && kind != "artist" && kind != "link" || !validText(note, 0, 2000) || !validText(title, 0, 160) || !validText(artist, 0, 160) {
		return 0, ErrInvalid
	}
	if title == "" {
		title = link.Title
	}
	if labels != nil {
		normalized, err := normalizeLabels(*labels)
		if err != nil {
			return 0, err
		}
		labels = &normalized
	}
	if len(title) > 160 {
		title = link.Provider + " link"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if group != 0 {
		var member int
		err = tx.QueryRowContext(ctx, "SELECT 1 FROM group_members WHERE group_id=? AND user_id=?", group, sender).Scan(&member)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrMissing
		}
		if err != nil {
			return 0, err
		}
		recipient = sender
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO media(original_url,provider,title,artist,media_type,video_id,canonical_key,kind) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(canonical_key) DO NOTHING`, link.Original, link.Provider, title, artist, link.Type, link.VideoID, key, kind)
	if err != nil {
		return 0, err
	}
	var mid int64
	var existingKind string
	err = tx.QueryRowContext(ctx, "SELECT id,kind FROM media WHERE canonical_key=?", key).Scan(&mid, &existingKind)
	if err != nil {
		return 0, err
	}
	if title != link.Title || artist != "" {
		if _, err = tx.ExecContext(ctx, "UPDATE media SET title=CASE WHEN title=original_url THEN ? ELSE title END,artist=CASE WHEN artist='' THEN ? ELSE artist END WHERE id=?", title, artist, mid); err != nil {
			return 0, err
		}
	}
	if existingKind == "link" && kind != "link" {
		if _, err = tx.ExecContext(ctx, "UPDATE media SET kind=? WHERE id=?", kind, mid); err != nil {
			return 0, err
		}
	}
	var groupValue any
	if group > 0 {
		groupValue = group
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO recommendations(media_id,sender_id,note,created_at,group_id,source_url,visibility) VALUES(?,?,?,?,?,?,?)", mid, sender, note, time.Now().Unix(), groupValue, raw, visibility)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO recommendation_destinations VALUES(?,?)", id, recipient); err != nil {
		return 0, err
	}
	if media.SupportsMetadata(raw) {
		if _, err = tx.ExecContext(ctx, "INSERT INTO metadata_jobs(media_id) SELECT ? WHERE NOT EXISTS(SELECT 1 FROM media_artwork WHERE media_id=?) ON CONFLICT DO NOTHING", mid, mid); err != nil {
			return 0, err
		}
	}
	if kind == "track" {
		if _, err = recording(ctx, tx, mid, key, raw, title, 0); err != nil {
			return 0, err
		}
	}
	if labels != nil {
		if err = saveLabels(ctx, tx, id, *labels); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}
func (s *Store) RecommendMusic(ctx context.Context, sender, recipient, group int64, raw, note, kind, title, artist string) (int64, error) {
	return s.recommend(ctx, sender, recipient, group, raw, note, kind, title, artist, "private", nil)
}

// ShareMusic explicitly posts to the signed-in instance, without individual gift delivery.
func (s *Store) ShareMusic(ctx context.Context, sender int64, raw, note, kind, title, artist string) (int64, error) {
	return s.recommend(ctx, sender, 0, 0, raw, note, kind, title, artist, "members", nil)
}

func (s *Store) Browse(ctx context.Context, viewer int64, history bool, f Filter, limit, offset int) ([]Recommendation, error) {
	f.Genre = strings.TrimSpace(f.Genre)
	f.Tag = strings.TrimSpace(f.Tag)
	if f.Genre != "" && !validLabel(f.Genre, 80) || f.Tag != "" && !validLabel(f.Tag, 40) {
		return nil, ErrInvalid
	}
	if limit < 1 || limit > 100 || offset < 0 || f.Person < 0 || f.Group < 0 || f.Status != "" && !validListening(f.Status) || f.Kind != "" && f.Kind != "track" && f.Kind != "album" && f.Kind != "artist" && f.Kind != "link" {
		return nil, ErrInvalid
	}
	where := visible
	args := []any{viewer, viewer, viewer, viewer, viewer}
	if f.Recent {
		where += ` AND r.visibility='members'`
	} else if !history {
		where += ` AND (r.group_id IS NOT NULL OR (r.visibility='private' AND d.user_id=?))`
		args = append(args, viewer)
	} else {
		where += ` AND (r.visibility='private' OR r.sender_id=? OR ms.user_id IS NOT NULL OR EXISTS(SELECT 1 FROM comments c WHERE c.recommendation_id=r.id AND c.author_id=?))`
		args = append(args, viewer, viewer)
	}
	if f.Status != "" {
		where += " AND coalesce(ms.listening,'unheard')=?"
		args = append(args, f.Status)
	}
	if f.Person > 0 {
		where += " AND r.sender_id=?"
		args = append(args, f.Person)
	}
	if f.Group > 0 {
		where += " AND r.group_id=?"
		args = append(args, f.Group)
	}
	if f.Kind != "" {
		where += " AND m.kind=?"
		args = append(args, f.Kind)
	}
	if f.Genre != "" {
		where += " AND l.genre_key=?"
		args = append(args, strings.ToLower(f.Genre))
	}
	if f.Tag != "" {
		where += " AND EXISTS(SELECT 1 FROM json_each(coalesce(l.tag_keys,'[]')) tags WHERE tags.value=?)"
		args = append(args, strings.ToLower(f.Tag))
	}
	order := "r.created_at DESC,r.id DESC"
	var preferred []any
	if f.UsePreferences {
		prefs, err := s.DiscoveryPreferences(ctx, viewer)
		if err != nil {
			return nil, err
		}
		where += " AND NOT " + preferredMatch
		args = append(args, labelJSON(prefs.ExcludedGenres, true), labelJSON(prefs.ExcludedTags, true))
		order = preferredMatch + " DESC," + order
		preferred = []any{labelJSON(prefs.PreferredGenres, true), labelJSON(prefs.PreferredTags, true)}
	}
	query := selectRecommendation + " WHERE " + where + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	if f.GroupByMusic {
		from := selectRecommendation[strings.Index(selectRecommendation, " FROM recommendations"):]
		musicOrder := "max(r.created_at) DESC,max(r.id) DESC"
		if f.UsePreferences {
			musicOrder = "max(" + preferredMatch + ") DESC," + musicOrder
		}
		query = selectRecommendation + " WHERE " + where + " AND r.media_id IN (SELECT r.media_id" + from + " WHERE " + where + " GROUP BY r.media_id ORDER BY " + musicOrder + " LIMIT ? OFFSET ?) ORDER BY " + order
		args = append(args, append([]any{}, args...)...)
	}
	args = append(args, preferred...)
	args = append(args, limit, offset)
	if f.GroupByMusic {
		args = append(args, preferred...)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Recommendation{}
	for rows.Next() {
		r, err := scanRecommendation(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}
func (s *Store) Groups(ctx context.Context, user int64) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT g.id,g.owner_id,g.name FROM groups g JOIN group_members m ON m.group_id=g.id WHERE m.user_id=? ORDER BY g.name", user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		var g Group
		if err = rows.Scan(&g.ID, &g.OwnerID, &g.Name); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
func (s *Store) SaveGroup(ctx context.Context, owner, id int64, name string, members []int64) (int64, error) {
	name = strings.TrimSpace(name)
	if !validText(name, 1, 80) || len(members) > 500 {
		return 0, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if id == 0 {
		result, err := tx.ExecContext(ctx, "INSERT INTO groups(owner_id,name) VALUES(?,?)", owner, name)
		if err != nil {
			return 0, err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return 0, err
		}
	} else {
		res, err := tx.ExecContext(ctx, "UPDATE groups SET name=? WHERE id=? AND owner_id=?", name, id, owner)
		if err = mutationResult(res, err); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM group_members WHERE group_id=?", id); err != nil {
			return 0, err
		}
	}
	members = append(members, owner)
	for _, u := range members {
		if _, err = tx.ExecContext(ctx, "INSERT INTO group_members VALUES(?,?) ON CONFLICT DO NOTHING", id, u); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}
func (s *Store) GroupMembers(ctx context.Context, viewer, id int64) ([]int64, error) {
	var yes int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM group_members WHERE group_id=? AND user_id=?", id, viewer).Scan(&yes); err != nil {
		return nil, ErrMissing
	}
	rows, err := s.db.QueryContext(ctx, "SELECT user_id FROM group_members WHERE group_id=? ORDER BY user_id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var u int64
		if err = rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) LinkDiscussion(ctx context.Context, viewer, id int64, raw string) error {
	if _, err := reference.URL(raw); err != nil {
		return ErrInvalid
	}
	r, err := s.Recommendation(ctx, viewer, id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO discussions VALUES(?,?,?) ON CONFLICT DO NOTHING", r.MediaID, viewer, raw)
	return err
}
func (s *Store) Discussions(ctx context.Context, viewer, id int64) ([]string, error) {
	r, err := s.Recommendation(ctx, viewer, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT url FROM discussions WHERE media_id=? AND user_id=? ORDER BY url", r.MediaID, viewer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}

func (s *Store) RemoveDiscussion(ctx context.Context, viewer, id int64, raw string) error {
	if _, err := reference.URL(raw); err != nil {
		return ErrInvalid
	}
	r, err := s.Recommendation(ctx, viewer, id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "DELETE FROM discussions WHERE media_id=? AND user_id=? AND url=?", r.MediaID, viewer, raw)
	return err
}
