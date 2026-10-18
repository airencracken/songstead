// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"testing/quick"
)

func TestSongLikesArePrivateAndSharedAcrossRecommendations(t *testing.T) {
	s, users := fixture(t)
	ctx := t.Context()
	first := recommend(t, s, users[0], users[1])
	second, err := s.ShareMusic(ctx, users[2], "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "Another recommendation", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.React(ctx, users[1], first, Reaction{"revisit", -1, "Private note"}); err != nil {
		t.Fatal(err)
	}
	for _, liked := range []bool{true, true, false, false, true} {
		if err := s.SetLiked(ctx, users[1], second, liked); err != nil {
			t.Fatal(err)
		}
		for _, id := range []int64{first, second} {
			for _, viewer := range users {
				if viewer == users[2] && id == first {
					continue
				}
				item, err := s.Recommendation(ctx, viewer, id)
				if err != nil || item.Liked != (liked && viewer == users[1]) {
					t.Fatal("song like not private and shared", item, err)
				}
				if viewer == users[1] && (item.Listening != "revisit" || id == first && (item.Rating != -1 || item.PersonalNote != "Private note")) {
					t.Fatal("liking changed listening feedback", item)
				}
			}
		}
	}
	// Saving notes and posting the same song preserve its like.
	if err := s.React(ctx, users[1], second, Reaction{"saved", 0, "Another note"}); err != nil {
		t.Fatal(err)
	}
	posted, err := s.ShareMusic(ctx, users[1], "https://youtu.be/dQw4w9WgXcQ?t=50", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	item, err := s.Recommendation(ctx, users[1], posted)
	if err != nil || !item.Liked || item.Listening != "listened" {
		t.Fatal("posting cleared song like", item, err)
	}
	archive, err := s.Export(ctx, User{ID: users[1]})
	if err != nil || archive.Version != 6 || len(archive.Recommendations) != 3 {
		t.Fatal("liked export", archive, err)
	}
	for _, item := range archive.Recommendations {
		if !item.Liked {
			t.Fatal("export lost own song like", item)
		}
	}
	other, err := s.Export(ctx, User{ID: users[2]})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range other.Recommendations {
		if item.Liked {
			t.Fatal("export exposed another person's like", item)
		}
	}
}

func TestSongLikePermissionsSchemaAndAtomicity(t *testing.T) {
	s, users := fixture(t)
	ctx := t.Context()
	id := recommend(t, s, users[0], users[1])
	for _, viewer := range []int64{users[2], 0, -1, 999999} {
		if err := s.SetLiked(ctx, viewer, id, true); !errors.Is(err, ErrMissing) {
			t.Fatal("unauthorized song like", viewer, err)
		}
	}
	for _, id := range []int64{0, -1, 999999} {
		if err := s.SetLiked(ctx, users[1], id, true); !errors.Is(err, ErrMissing) {
			t.Fatal("unknown recommendation liked", id, err)
		}
	}
	if _, err := s.db.Exec("UPDATE users SET suspended=1 WHERE id=?", users[1]); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLiked(ctx, users[1], id, true); !errors.Is(err, ErrMissing) {
		t.Fatal("suspended member changed song like", err)
	}
	if _, err := s.db.Exec("UPDATE users SET suspended=0 WHERE id=?", users[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("CREATE TRIGGER refuse_like BEFORE INSERT ON music_states BEGIN SELECT RAISE(ABORT,'test failure'); END"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLiked(ctx, users[1], id, true); err == nil {
		t.Fatal("failed like accepted")
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM music_states WHERE user_id=?", users[1]).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed like created state", count, err)
	}
	if _, err := s.db.Exec("DROP TRIGGER refuse_like"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLiked(ctx, users[1], id, true); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []any{-1, 2, "yes", nil} {
		if _, err := s.db.Exec("UPDATE music_states SET liked=? WHERE user_id=?", invalid, users[1]); err == nil {
			t.Fatal("invalid like accepted by schema", invalid)
		}
	}
	if _, err := s.db.Exec("CREATE TRIGGER refuse_unlike BEFORE UPDATE ON music_states BEGIN SELECT RAISE(ABORT,'test failure'); END"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLiked(ctx, users[1], id, false); err == nil {
		t.Fatal("failed unlike accepted")
	}
	item, err := s.Recommendation(ctx, users[1], id)
	if err != nil || !item.Liked || item.Listening != "unheard" {
		t.Fatal("failed unlike changed song", item, err)
	}
}

func TestLikedSongFilterPaginationAndURLProperties(t *testing.T) {
	s, users := fixture(t)
	ctx := t.Context()
	var likedIDs []int64
	property := func(seed uint8) bool {
		i := len(likedIDs)
		raw := fmt.Sprintf("https://music.example/song/%d", i)
		// The same title on different URLs must remain different songs.
		first, err := s.ShareMusic(ctx, users[0], raw, "first", "track", "Same title", "Same artist")
		if err != nil {
			return false
		}
		second, err := s.ShareMusic(ctx, users[2], raw+"#position", "second", "track", "Same title", "Same artist")
		if err != nil || s.SetLiked(ctx, users[1], first, true) != nil {
			return false
		}
		likedIDs = append(likedIDs, second)
		limit := int(seed%5) + 1
		var seen int
		for offset := 0; offset < len(likedIDs); offset += limit {
			rows, err := s.Browse(ctx, users[1], false, Filter{Liked: true, GroupByMusic: true}, limit, offset)
			if err != nil || len(rows) != 2*min(limit, len(likedIDs)-offset) {
				return false
			}
			for i := 0; i < len(rows); i += 2 {
				if !rows[i].Liked || rows[i].ID != likedIDs[len(likedIDs)-1-offset-i/2] || rows[i].MediaID != rows[i+1].MediaID {
					return false
				}
				seen++
			}
		}
		other, err := s.Browse(ctx, users[0], false, Filter{Recent: true, Liked: true}, 100, 0)
		return err == nil && len(other) == 0 && seen == len(likedIDs)
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 15}); err != nil {
		t.Fatal(err)
	}
}

func TestSongLikesMigrationAndSnapshots(t *testing.T) {
	s, users := fixture(t)
	ctx := t.Context()
	first := recommend(t, s, users[0], users[1])
	second := recommend(t, s, users[0], users[1])
	for i, reaction := range []Reaction{{"saved", 1, "older positive"}, {"revisit", -1, "newer negative"}} {
		if err := s.React(ctx, users[1], []int64{first, second}[i], reaction); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.React(ctx, users[0], first, Reaction{"listened", 1, "sender positive"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE reactions SET updated_at=recommendation_id; ALTER TABLE recommendations DROP COLUMN title_override; ALTER TABLE recommendations DROP COLUMN artist_override; ALTER TABLE recommendations DROP COLUMN kind_override; DROP INDEX music_states_liked; ALTER TABLE music_states DROP COLUMN liked; PRAGMA user_version=8"); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(t.TempDir(), "old.db")
	if err := s.Backup(ctx, old); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshot(ctx, old); err != nil {
		t.Fatal("schema 8 snapshot rejected", err)
	}
	if current, err := OpenCurrent(old); err == nil {
		current.Close()
		t.Fatal("account command migrated old schema")
	}
	upgraded, err := Open(old)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	for i, viewer := range users[:2] {
		item, err := upgraded.Recommendation(ctx, viewer, first)
		if err != nil || item.Liked != (i == 0) {
			t.Fatal("latest rating was not migrated privately", item, err)
		}
		if i == 1 && (item.Listening != "revisit" || item.PersonalNote != "older positive" || item.Rating != 1) {
			t.Fatal("migration changed existing feedback", item)
		}
	}
	if err := ValidateSnapshot(ctx, old); err != nil {
		t.Fatal("upgraded snapshot rejected", err)
	}
	if _, err := upgraded.db.Exec("PRAGMA ignore_check_constraints=ON; UPDATE music_states SET liked=2"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshot(ctx, old); err == nil {
		t.Fatal("corrupt like snapshot accepted")
	}
	// A version claiming likes without the column cannot pass validation.
	if err := upgraded.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", old)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("DROP INDEX music_states_liked; ALTER TABLE music_states DROP COLUMN liked"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshot(ctx, old); err == nil {
		t.Fatal("forged schema 9 snapshot accepted")
	}
}

func TestSongLikeMigrationFailureIsAtomic(t *testing.T) {
	s, users := fixture(t)
	id := recommend(t, s, users[0], users[1])
	if err := s.React(t.Context(), users[1], id, Reaction{"saved", 1, "Keep this note"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`ALTER TABLE recommendations DROP COLUMN title_override; ALTER TABLE recommendations DROP COLUMN artist_override; ALTER TABLE recommendations DROP COLUMN kind_override; DROP INDEX music_states_liked; ALTER TABLE music_states DROP COLUMN liked;
 PRAGMA user_version=8;
 CREATE TRIGGER refuse_migration BEFORE UPDATE ON music_states BEGIN SELECT RAISE(ABORT,'test failure'); END;`); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "old.db")
	if err := s.Backup(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	if opened, err := Open(path); err == nil {
		opened.Close()
		t.Fatal("failed migration accepted")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version, columns int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 8 {
		t.Fatal("failed migration changed version", version, err)
	}
	if err := db.QueryRow("SELECT count(*) FROM pragma_table_info('music_states') WHERE name='liked'").Scan(&columns); err != nil || columns != 0 {
		t.Fatal("failed migration left a partial schema", columns, err)
	}
	var note string
	if err := db.QueryRow("SELECT note FROM reactions WHERE recommendation_id=? AND user_id=?", id, users[1]).Scan(&note); err != nil || note != "Keep this note" {
		t.Fatal("failed migration changed personal feedback", note, err)
	}
}
