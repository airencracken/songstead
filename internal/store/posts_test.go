// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostEditingIsolationAndDeletion(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	id := recommend(t, s, u[0], u[1])
	duplicate := recommend(t, s, u[1], u[0])
	original, err := s.Recommendation(ctx, u[0], id)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.React(ctx, u[1], id, Reaction{"revisit", 1, "Keep private note"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLiked(ctx, u[1], id, true); err != nil {
		t.Fatal(err)
	}
	if err := s.AddComment(ctx, u[1], id, "At 0:10 this is great"); err != nil {
		t.Fatal(err)
	}
	edit := PostEdit{URL: original.URL, Title: "Netherworlds", Artist: "Insomnium", Kind: "album", Note: "Corrected", Labels: Labels{Genre: "Metal", Tags: []string{"new"}}}
	for _, actor := range []int64{u[1], u[2], 0} {
		if err := s.EditPost(ctx, actor, id, edit); !errors.Is(err, ErrMissing) {
			t.Fatal("unauthorized edit", actor, err)
		}
		if err := s.DeletePost(ctx, actor, id); !errors.Is(err, ErrMissing) {
			t.Fatal("unauthorized delete", actor, err)
		}
	}
	if err := s.EditPost(ctx, u[0], id, edit); err != nil {
		t.Fatal(err)
	}
	got, err := s.Recommendation(ctx, u[1], id)
	if err != nil || got.Title != edit.Title || got.Artist != edit.Artist || got.Kind != "album" || got.CreatedAt != original.CreatedAt || got.Visibility != original.Visibility || got.SenderID != u[0] || got.RecipientID != u[1] || got.PersonalNote != "Keep private note" || !got.Liked {
		t.Fatal(got, err)
	}
	other, err := s.Recommendation(ctx, u[1], duplicate)
	if err != nil || other.Title != original.Title || other.Kind != original.Kind || other.Artist != original.Artist {
		t.Fatal("edit changed another post", other, err)
	}
	matches, err := s.SearchMusic(ctx, u[1], "Netherworlds", 0, 20)
	if err != nil || len(matches) != 1 || matches[0].ID != id {
		t.Fatal("search missed corrected title", matches, err)
	}
	albums, err := s.Browse(ctx, u[1], false, Filter{Kind: "album"}, 20, 0)
	if err != nil || len(albums) != 1 || albums[0].ID != id {
		t.Fatal("kind filter missed edit", albums, err)
	}
	edit.URL = "https://example.org/another-album"
	if err := s.EditPost(ctx, u[0], id, edit); !errors.Is(err, ErrInvalid) {
		t.Fatal("moved timestamped conversation", err)
	}
	if err := s.DeletePost(ctx, u[0], id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Recommendation(ctx, u[1], id); !errors.Is(err, ErrMissing) {
		t.Fatal("deleted post readable", err)
	}
	other, err = s.Recommendation(ctx, u[1], duplicate)
	if err != nil || !other.Liked || other.Listening != "revisit" {
		t.Fatal("deleted shared music state", other, err)
	}
	for _, table := range []string{"comments", "annotation_offsets", "reactions", "recommendation_labels", "recommendation_destinations"} {
		var count int
		if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 1 && table == "recommendation_destinations" || count != 0 && table != "recommendation_destinations" {
			t.Fatal("post children remain", table, count, err)
		}
	}
}

func TestPostEditsRollbackAndSnapshots(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	id := recommend(t, s, u[0], u[1])
	original, err := s.Recommendation(ctx, u[0], id)
	if err != nil {
		t.Fatal(err)
	}
	edit := PostEdit{URL: "https://example.org/album", Title: "Corrected", Artist: "Artist", Kind: "album", Labels: Labels{Genre: "Metal"}}
	if _, err := s.db.Exec(`CREATE TRIGGER refuse_post_labels BEFORE INSERT ON recommendation_labels BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.EditPost(ctx, u[0], id, edit); err == nil {
		t.Fatal("failed write succeeded")
	}
	got, err := s.Recommendation(ctx, u[0], id)
	if err != nil || got.URL != original.URL || got.Title != original.Title {
		t.Fatal("partial edit persisted", got, err)
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM media").Scan(&count); err != nil || count != 1 {
		t.Fatal("failed edit left music behind", count, err)
	}
	if _, err := s.db.Exec("DROP TRIGGER refuse_post_labels"); err != nil {
		t.Fatal(err)
	}
	invalid := edit
	invalid.Title = strings.Repeat("x", 161)
	if err := s.EditPost(ctx, u[0], id, invalid); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid title accepted", err)
	}
	if err := s.EditPost(ctx, u[0], id, edit); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "snapshot.db")
	if err := s.Backup(ctx, path); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshot(ctx, path); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	got, err = restored.Recommendation(ctx, u[0], id)
	if err != nil || got.Title != edit.Title || got.URL != edit.URL {
		t.Fatal("snapshot lost edit", got, err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER refuse_post_delete BEFORE DELETE ON comments BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.AddComment(ctx, u[1], id, "Keep comment"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePost(ctx, u[0], id); err == nil {
		t.Fatal("failed deletion succeeded")
	}
	if _, err := s.Recommendation(ctx, u[0], id); err != nil {
		t.Fatal("partial deletion", err)
	}
	comments, err := s.Comments(ctx, u[0], id)
	if err != nil || len(comments) != 1 {
		t.Fatal("failed delete lost comments", comments, err)
	}
}

func TestPostAuthorsAcrossAudiences(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	group, err := s.SaveGroup(ctx, u[0], 0, "Music friends", []int64{u[1]})
	if err != nil {
		t.Fatal(err)
	}
	private := recommend(t, s, u[0], u[1])
	shared, err := s.ShareMusic(ctx, u[0], "https://example.org/shared", "Shared note", "", "Shared", "Artist")
	if err != nil {
		t.Fatal(err)
	}
	groupPost, err := s.RecommendMusic(ctx, u[0], 0, group, "https://example.org/group", "Group note", "", "Group", "Artist")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{private, shared, groupPost} {
		item, err := s.Recommendation(ctx, u[0], id)
		if err != nil {
			t.Fatal(err)
		}
		edit := PostEdit{URL: item.URL, Title: "Updated", Kind: "album"}
		if _, err := s.db.Exec("UPDATE users SET suspended=1 WHERE id=?", u[0]); err != nil {
			t.Fatal(err)
		}
		if err := s.EditPost(ctx, u[0], id, edit); !errors.Is(err, ErrMissing) {
			t.Fatal("suspended author edited", err)
		}
		if err := s.DeletePost(ctx, u[0], id); !errors.Is(err, ErrMissing) {
			t.Fatal("suspended author deleted", err)
		}
		if _, err := s.db.Exec("UPDATE users SET suspended=0 WHERE id=?", u[0]); err != nil {
			t.Fatal(err)
		}
		if err := s.EditPost(ctx, u[0], id, edit); err != nil {
			t.Fatal(err)
		}
		got, err := s.Recommendation(ctx, u[1], id)
		if err != nil || got.Title != "Updated" || got.GroupID != item.GroupID || got.Visibility != item.Visibility || got.RecipientID != item.RecipientID {
			t.Fatal("audience changed", got, err)
		}
		if err := s.DeletePost(ctx, u[0], id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPostEditsSchemaNineUpgrade(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	raw := "https://youtube.com/playlist?list=OLAK5uy_mEf5zzrxJlXov2xQ1Ph7o0cgPF7XfSJ00&si=share"
	id, err := s.ShareMusic(ctx, u[0], raw, "Keep note", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.Recommendation(ctx, u[1], id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`ALTER TABLE recommendations DROP COLUMN title_override; ALTER TABLE recommendations DROP COLUMN artist_override; ALTER TABLE recommendations DROP COLUMN kind_override; DELETE FROM metadata_jobs; PRAGMA user_version=9`); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "schema9.db")
	if err := s.Backup(ctx, path); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshot(ctx, path); err != nil {
		t.Fatal("legacy snapshot rejected", err)
	}
	if current, err := OpenCurrent(path); err == nil {
		current.Close()
		t.Fatal("account command migrated schema")
	}
	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	got, err := upgraded.Recommendation(ctx, u[1], id)
	if err != nil || got.Title != before.Title || got.Note != before.Note || got.CreatedAt != before.CreatedAt {
		t.Fatal("migration lost post", got, err)
	}
	pending, err := upgraded.PreviewPending(ctx, u[1], id)
	if err != nil || !pending {
		t.Fatal("old playlist was not queued", pending, err)
	}
	if err := upgraded.EditPost(ctx, u[0], id, PostEdit{URL: raw, Title: "Netherworlds", Kind: "album"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshot(ctx, path); err != nil {
		t.Fatal("new snapshot rejected", err)
	}
	if _, err := upgraded.db.Exec("ALTER TABLE recommendations DROP COLUMN kind_override"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshot(ctx, path); err == nil {
		t.Fatal("incomplete edit schema accepted")
	}
}
