// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"testing/quick"
)

func TestRecentAudienceIsolationAndPrivateState(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	group, err := s.SaveGroup(ctx, u[0], 0, "Just us", []int64{u[1]})
	if err != nil {
		t.Fatal(err)
	}
	shared, err := s.ShareMusic(ctx, u[0], "https://youtu.be/dQw4w9WgXcQ", "For everyone", "track", "Music", "Artist")
	if err != nil {
		t.Fatal(err)
	}
	private, err := s.RecommendMusic(ctx, u[2], u[0], 0, "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "Private gift", "track", "Music", "Artist")
	if err != nil {
		t.Fatal(err)
	}
	groupGift, err := s.RecommendMusic(ctx, u[0], 0, group, "https://youtu.be/dQw4w9WgXcQ?t=30", "Group gift", "track", "Music", "Artist")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.ShareMusic(ctx, u[2], "https://youtu.be/dQw4w9WgXcQ?t=40", "Another perspective", "track", "Music", "Artist")
	if err != nil {
		t.Fatal(err)
	}
	for _, viewer := range u {
		for _, grouped := range []bool{false, true} {
			limit := 10
			if grouped {
				limit = 1
			}
			rows, err := s.Browse(ctx, viewer, false, Filter{Recent: true, GroupByMusic: grouped}, limit, 0)
			if err != nil || len(rows) != 2 || rows[0].ID != second || rows[1].ID != shared || rows[0].MediaID != rows[1].MediaID {
				t.Fatalf("Recent mixed audiences or lost a shared context for %d: %+v %v", viewer, rows, err)
			}
			for _, row := range rows {
				if row.Visibility != "members" || row.GroupID != 0 || row.Note == "Private gift" || row.Note == "Group gift" {
					t.Fatal("private provenance in Recent", row)
				}
			}
		}
	}
	if err := s.React(ctx, u[1], shared, Reaction{"saved", 1, "My private thoughts"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddComment(ctx, u[1], shared, "A shared thought at 0:10"); err != nil {
		t.Fatal(err)
	}
	for _, viewer := range []int64{u[0], u[2]} {
		item, err := s.Recommendation(ctx, viewer, shared)
		if err != nil || item.PersonalNote != "" || item.Rating != 0 || item.Listening != "unheard" {
			t.Fatal("shared post disclosed someone else's private state", item, err)
		}
		comments, err := s.Comments(ctx, viewer, shared)
		if err != nil || len(comments) != 1 || len(comments[0].Offsets) != 1 {
			t.Fatal("shared conversation inaccessible", comments, err)
		}
	}
	rows, err := s.Browse(ctx, u[1], false, Filter{Recent: true, Status: "saved", Person: u[2], Kind: "track"}, 10, 0)
	if err != nil || len(rows) != 1 || rows[0].ID != second || rows[0].PersonalNote != "" {
		t.Fatal("Recent filters or per-context personal notes", rows, err)
	}
	rows, err = s.Browse(ctx, u[2], false, Filter{Recent: true, Status: "saved"}, 10, 0)
	if err != nil || len(rows) != 0 {
		t.Fatal("private organization filter leaked", rows, err)
	}
	rows, err = s.Browse(ctx, u[1], false, Filter{}, 10, 0)
	if err != nil || len(rows) != 1 || rows[0].ID != groupGift {
		t.Fatal("shared posts delivered as inbox assignments", rows, err)
	}
	rows, err = s.Browse(ctx, u[2], true, Filter{}, 10, 0)
	if err != nil || len(rows) != 2 || rows[0].ID != second || rows[1].ID != private {
		t.Fatal("untouched shared posts inserted into personal history", rows, err)
	}
	for _, viewer := range []int64{0, -1, 999999} {
		if rows, err := s.Browse(ctx, viewer, false, Filter{Recent: true}, 10, 0); err != nil || len(rows) != 0 {
			t.Fatal("Recent accessible without an account", viewer, rows, err)
		}
		if _, err := s.Recommendation(ctx, viewer, shared); !errors.Is(err, ErrMissing) {
			t.Fatal("shared detail accessible without an account", viewer, err)
		}
		if _, err := s.Comments(ctx, viewer, shared); !errors.Is(err, ErrMissing) {
			t.Fatal("shared comments accessible without an account", viewer, err)
		}
		if err := s.AddComment(ctx, viewer, shared, "intruder"); !errors.Is(err, ErrMissing) {
			t.Fatal("shared comment write allowed without account", viewer, err)
		}
		if err := s.React(ctx, viewer, shared, Reaction{"saved", 1, "intruder"}); !errors.Is(err, ErrMissing) {
			t.Fatal("shared state write allowed without account", viewer, err)
		}
	}
	if err := s.AddComment(ctx, u[1], private, "intruder"); !errors.Is(err, ErrMissing) {
		t.Fatal("private write allowed alongside shared duplicate", err)
	}
	if _, err := s.Recommendation(ctx, u[2], groupGift); !errors.Is(err, ErrMissing) {
		t.Fatal("group detail broadened alongside shared duplicate", err)
	}
	archive, err := s.Export(ctx, User{ID: u[2], Username: "carol"})
	if err != nil || len(archive.Recommendations) != 3 {
		t.Fatal("export visibility", archive, err)
	}
	for _, item := range archive.Recommendations {
		if item.ID == groupGift || item.PersonalNote != "" || item.Listening != "unheard" {
			t.Fatal("export leaks private context/state", item)
		}
	}
}

func TestRecentPaginationIgnoresPrivatePosts(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	var shared []int64
	for i := 0; i < 54; i++ {
		id, err := s.ShareMusic(ctx, u[i%2], fmt.Sprintf("https://music.example/shared/%d", i), "Shared", "album", "Album", "")
		if err != nil {
			t.Fatal(err)
		}
		shared = append(shared, id)
		if _, err := s.Recommend(ctx, u[0], u[1], fmt.Sprintf("https://music.example/private/%d", i), "Secret"); err != nil {
			t.Fatal(err)
		}
	}
	for _, grouped := range []bool{false, true} {
		for _, page := range []struct{ offset, count int }{{0, 50}, {50, 4}, {54, 0}} {
			rows, err := s.Browse(ctx, u[2], false, Filter{Recent: true, GroupByMusic: grouped}, 50, page.offset)
			if err != nil || len(rows) != page.count {
				t.Fatal("wrong Recent page", page, len(rows), err)
			}
			for i, row := range rows {
				if row.ID != shared[len(shared)-1-page.offset-i] || row.Visibility != "members" {
					t.Fatal("pagination skipped or disclosed a recommendation", row)
				}
			}
		}
	}
}

func TestRecentSchemaAudienceConstraints(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	group, err := s.SaveGroup(ctx, u[0], 0, "Private group", nil)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.RecommendMusic(ctx, u[0], 0, group, "https://music.example/album", "Private", "album", "Album", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{"members", "public", "", nil} {
		if _, err := s.db.Exec("UPDATE recommendations SET visibility=? WHERE id=?", value, id); err == nil {
			t.Fatal("schema accepted invalid group audience", value)
		}
	}
	shared, err := s.ShareMusic(ctx, u[0], "https://music.example/shared", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE recommendations SET group_id=? WHERE id=?", group, shared); err == nil {
		t.Fatal("shared row accepted private group context")
	}
	item, err := s.Recommendation(ctx, u[0], id)
	if err != nil || item.Visibility != "private" {
		t.Fatal("failed update changed privacy", item, err)
	}
}

func TestRecentSharingAtomicity(t *testing.T) {
	s, u := fixture(t)
	if _, err := s.db.Exec(`CREATE TRIGGER fail_destination BEFORE INSERT ON recommendation_destinations BEGIN SELECT RAISE(ABORT,'cannot deliver'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ShareMusic(t.Context(), u[0], "https://youtu.be/dQw4w9WgXcQ", "Draft", "track", "Title", ""); err == nil {
		t.Fatal("accepted failed destination")
	}
	for _, table := range []string{"media", "recommendations", "recommendation_destinations", "recordings", "media_recordings", "metadata_jobs"} {
		var count int
		if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("partial shared recommendation", table, count, err)
		}
	}
}

func TestRecentMigrationKeepsExistingGiftsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songstead.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	u := make([]int64, 3)
	for i, name := range []string{"alice", "bobby", "carol"} {
		u[i], err = s.CreateUser(t.Context(), name, "a-long-test-password")
		if err != nil {
			t.Fatal(err)
		}
	}
	id := recommend(t, s, u[0], u[1])
	group, err := s.SaveGroup(t.Context(), u[0], 0, "Our group", []int64{u[1]})
	if err != nil {
		t.Fatal(err)
	}
	groupID, err := s.RecommendMusic(t.Context(), u[0], 0, group, "https://music.example/album", "Old group note", "album", "Album", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DROP INDEX recommendations_recent; ALTER TABLE recommendations DROP COLUMN visibility; DROP TABLE user_profiles; DROP TABLE media_artwork; DROP TABLE recommendation_labels; DROP TABLE discovery_preferences; DROP TABLE branding_assets; DROP TABLE password_resets; DROP TABLE invitations; DROP TABLE instance_settings; ALTER TABLE users DROP COLUMN invited_by; ALTER TABLE users DROP COLUMN can_invite; ALTER TABLE users DROP COLUMN suspended; ALTER TABLE users DROP COLUMN role; PRAGMA user_version=2;`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshot(t.Context(), path); err == nil {
		t.Fatal("accepted pre-migration snapshot")
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, gift := range []int64{id, groupID} {
		item, err := s.Recommendation(t.Context(), u[1], gift)
		if err != nil || item.Visibility != "private" {
			t.Fatal("migration widened old gift's audience", item, err)
		}
		if _, err := s.Recommendation(t.Context(), u[2], gift); !errors.Is(err, ErrMissing) {
			t.Fatal("old gift disclosed after migration", err)
		}
	}
	for _, viewer := range u {
		rows, err := s.Browse(t.Context(), viewer, false, Filter{Recent: true}, 10, 0)
		if err != nil || len(rows) != 0 {
			t.Fatal("old gifts automatically posted in Recent", rows, err)
		}
	}
	if err := ValidateSnapshot(t.Context(), path); err != nil {
		t.Fatal("migrated snapshot rejected", err)
	}
}

func TestRecentAudienceProperties(t *testing.T) {
	s, u := fixture(t)
	var serial int
	sharedIDs := map[int64]bool{}
	property := func(seed uint8) bool {
		serial++
		shared := seed%2 == 0
		var id int64
		var err error
		raw := fmt.Sprintf("https://music.example/property/%d", serial)
		if shared {
			id, err = s.ShareMusic(t.Context(), u[0], raw, "A gift", "", "", "")
		} else {
			id, err = s.Recommend(t.Context(), u[0], u[1], raw, "Private gift")
		}
		if err != nil {
			return false
		}
		if shared {
			sharedIDs[id] = true
		}
		for _, viewer := range u {
			rows, err := s.Browse(t.Context(), viewer, false, Filter{Recent: true}, 100, 0)
			if err != nil || len(rows) != len(sharedIDs) {
				return false
			}
			for _, row := range rows {
				if !sharedIDs[row.ID] || row.Visibility != "members" {
					return false
				}
			}
		}
		return true
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 30}); err != nil {
		t.Fatal(err)
	}
}
