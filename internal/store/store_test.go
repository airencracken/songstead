// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/quick"
	"time"

	"github.com/airencracken/comfylib/token"
	"github.com/airencracken/songstead/internal/media"
)

func fixture(t *testing.T) (*Store, []int64) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "songstead.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	var users []int64
	for _, name := range []string{"alice", "bobby", "carol"} {
		id, err := s.CreateUser(context.Background(), name, "a-long-test-password")
		if err != nil {
			t.Fatal(err)
		}
		users = append(users, id)
	}
	return s, users
}
func recommend(t *testing.T, s *Store, from, to int64) int64 {
	t.Helper()
	id, err := s.Recommend(context.Background(), from, to, "https://youtu.be/dQw4w9WgXcQ?t=30", "a song for your walk")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRecommendationsAndIndependentReactions(t *testing.T) {
	s, u := fixture(t)
	ctx := context.Background()
	id := recommend(t, s, u[0], u[1])
	second := recommend(t, s, u[0], u[1])
	if id == second {
		t.Fatal("distinct recommendations merged")
	}
	item, err := s.Recommendation(ctx, u[1], id)
	if err != nil || item.Listening != "unheard" || item.Rating != 0 || item.URL != "https://youtu.be/dQw4w9WgXcQ?t=30" {
		t.Fatalf("initial item: %+v %v", item, err)
	}
	if err := s.React(ctx, u[1], id, Reaction{"listened", 1, "a private thought"}); err != nil {
		t.Fatal(err)
	}
	if err := s.React(ctx, u[1], id, Reaction{"revisit", -1, "changed my mind"}); err != nil {
		t.Fatal(err)
	}
	item, err = s.Recommendation(ctx, u[1], id)
	if err != nil || item.Listening != "revisit" || item.Rating != -1 || item.PersonalNote != "changed my mind" {
		t.Fatalf("revision: %+v %v", item, err)
	}
	other, err := s.Recommendation(ctx, u[0], id)
	if err != nil || other.Listening != "unheard" || other.Rating != 0 || other.PersonalNote != "" {
		t.Fatal("reaction leaked to sender", other, err)
	}
	duplicate, err := s.Recommendation(ctx, u[1], second)
	if err != nil || duplicate.Listening != "revisit" || duplicate.MediaID != item.MediaID || duplicate.PersonalNote != "" {
		t.Fatal("duplicate music did not share private state")
	}
	items, err := s.List(ctx, u[1], false, "", 50, 0)
	if err != nil || len(items) != 2 || items[0].ID != second {
		t.Fatalf("arrival order: %+v %v", items, err)
	}
	if err := s.AddComment(ctx, u[0], id, "try it on headphones"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddComment(ctx, u[1], id, "I did; thanks!"); err != nil {
		t.Fatal(err)
	}
	comments, err := s.Comments(ctx, u[0], id)
	if err != nil || len(comments) != 2 || comments[0].Author != "alice" || comments[1].Body != "I did; thanks!" {
		t.Fatalf("comments: %+v %v", comments, err)
	}
}

func TestPermissions(t *testing.T) {
	s, u := fixture(t)
	ctx := context.Background()
	id := recommend(t, s, u[0], u[1])
	for _, viewer := range []int64{u[2], 0, 999999} {
		if _, err := s.Recommendation(ctx, viewer, id); !errors.Is(err, ErrMissing) {
			t.Fatalf("detail accessible to %d: %v", viewer, err)
		}
		if _, err := s.Comments(ctx, viewer, id); !errors.Is(err, ErrMissing) {
			t.Fatal("comments exposed", err)
		}
		if err := s.AddComment(ctx, viewer, id, "intruder"); !errors.Is(err, ErrMissing) {
			t.Fatal("unauthorized comment", err)
		}
		if err := s.React(ctx, viewer, id, Reaction{"listened", 1, "intruder"}); !errors.Is(err, ErrMissing) {
			t.Fatal("unauthorized reaction", err)
		}
		for _, history := range []bool{false, true} {
			list, err := s.List(ctx, viewer, history, "", 50, 0)
			if err != nil || len(list) != 0 {
				t.Fatal("private recommendation leaked in listing", list, err)
			}
		}
	}
}

func TestRecommendationAtomicity(t *testing.T) {
	s, u := fixture(t)
	ctx := context.Background()
	for _, table := range []string{"recommendation_destinations", "metadata_jobs"} {
		if _, err := s.db.Exec("CREATE TRIGGER refuse_insert BEFORE INSERT ON " + table + " BEGIN SELECT RAISE(ABORT,'injected failure'); END"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Recommend(ctx, u[0], u[1], "https://youtu.be/dQw4w9WgXcQ", "note"); err == nil {
			t.Fatal("injected failure accepted")
		}
		for _, target := range []string{"media", "recommendations", "recommendation_destinations", "metadata_jobs"} {
			var n int
			if err := s.db.QueryRow("SELECT count(*) FROM " + target).Scan(&n); err != nil || n != 0 {
				t.Fatal("partial submission survived", target, n, err)
			}
		}
		if _, err := s.db.Exec("DROP TRIGGER refuse_insert"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Recommend(ctx, u[0], 99999, "https://example.org/song", ""); err == nil {
		t.Fatal("missing recipient accepted")
	}
	var count int
	s.db.QueryRow("SELECT count(*) FROM media").Scan(&count)
	if count != 0 {
		t.Fatal("orphaned media")
	}
}

func TestSchemaConstraints(t *testing.T) {
	s, u := fixture(t)
	id := recommend(t, s, u[0], u[1])
	queries := []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO users(username,password_hash,created_at) VALUES('ALICE','hash',1)", nil},
		{"INSERT INTO sessions(token_hash,user_id,expires_at) VALUES('short',?,1)", []any{u[0]}},
		{"INSERT INTO reactions VALUES(?,?, 'imaginary',0,'',1)", []any{id, u[1]}},
		{"INSERT INTO reactions VALUES(?,?, 'listened',2,'',1)", []any{id, u[1]}},
		{"INSERT INTO reactions VALUES(999999,?,'listened',0,'',1)", []any{u[1]}},
		{"INSERT INTO comments(recommendation_id,author_id,body,created_at) VALUES(?,?,'',1)", []any{id, u[0]}},
	}
	for _, q := range queries {
		if _, err := s.db.Exec(q.sql, q.args...); err == nil {
			t.Fatal("schema accepted invalid row:", q.sql)
		}
	}
}

func TestMigrationRepeatabilityAndNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songstead.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.CreateUser(context.Background(), "alice", "a-long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	user, _, err := s.Credentials(context.Background(), "alice")
	if err != nil || user.ID != id {
		t.Fatal("migration changed data")
	}
	if _, err := s.db.Exec("PRAGMA user_version=9"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := Open(path); err == nil {
		t.Fatal("opened newer schema")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 9 {
		t.Fatal("modified newer schema")
	}
}

func TestReactionPropertiesAndAdversarialInput(t *testing.T) {
	s, u := fixture(t)
	ctx := context.Background()
	id := recommend(t, s, u[0], u[1])
	statuses := []string{"unheard", "listened", "revisit", "dismissed"}
	check := func(status, rating uint8) bool {
		reaction := Reaction{statuses[int(status)%4], int(rating)%3 - 1, "notes"}
		if err := s.React(ctx, u[1], id, reaction); err != nil {
			return false
		}
		got, err := s.Recommendation(ctx, u[1], id)
		return err == nil && got.Listening == reaction.Listening && got.Rating == reaction.Rating
	}
	if err := quick.Check(check, &quick.Config{MaxCount: 30}); err != nil {
		t.Fatal(err)
	}
	for _, reaction := range []Reaction{{"bad", 0, ""}, {"unheard", 2, ""}, {"listened", 0, strings.Repeat("x", 2001)}, {"listened", 0, "\x00"}, {"listened", 0, "\xff"}} {
		if err := s.React(ctx, u[1], id, reaction); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid reaction accepted", err)
		}
	}
	for _, body := range []string{"", "  \n", strings.Repeat("x", 2001), "\x00", "\xff"} {
		if err := s.AddComment(ctx, u[0], id, body); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid comment accepted", err)
		}
	}
	for _, name := range []string{"ab", "a b", "alice'; DROP TABLE users;--", "\xff", strings.Repeat("a", 25)} {
		if _, err := s.CreateUser(ctx, name, "a-long-test-password"); err == nil {
			t.Fatal("bad username accepted")
		}
	}
	for _, password := range []string{"short", strings.Repeat("a", 73), "invalid\xfflong-password"} {
		if err := ValidatePassword(password); err == nil {
			t.Fatal("bad password accepted")
		}
	}
}

func TestMetadataFailurePersistenceAndRecovery(t *testing.T) {
	s, u := fixture(t)
	ctx := context.Background()
	id := recommend(t, s, u[0], u[1])
	if err := s.Metadata(ctx, func(context.Context, string) (media.Metadata, error) { return media.Metadata{}, errors.New("offline") }); err != nil {
		t.Fatal(err)
	}
	item, err := s.Recommendation(ctx, u[1], id)
	if err != nil || item.Title != item.URL {
		t.Fatal("failure destroyed saved link", item, err)
	}
	var attempts int
	s.db.QueryRow("SELECT attempts FROM metadata_jobs").Scan(&attempts)
	if attempts != 1 {
		t.Fatal("retry not persisted")
	}
	s.db.Exec("UPDATE metadata_jobs SET next_attempt=0")
	if err := s.Metadata(ctx, func(_ context.Context, video string) (media.Metadata, error) {
		if video != "https://youtu.be/dQw4w9WgXcQ?t=30" {
			t.Fatal("wrong video")
		}
		return media.Metadata{Title: "A good song", Artist: "Artist", Thumbnail: "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg", Artwork: artworkFixture(t)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	item, err = s.Recommendation(ctx, u[1], id)
	if err != nil || item.Title != "A good song" || item.URL != "https://youtu.be/dQw4w9WgXcQ?t=30" {
		t.Fatal("metadata lost original URL")
	}
	var jobs int
	s.db.QueryRow("SELECT count(*) FROM metadata_jobs").Scan(&jobs)
	if jobs != 0 {
		t.Fatal("successful job retained")
	}
}

func TestMetadataRetriesAreBoundedAndCancelled(t *testing.T) {
	s, u := fixture(t)
	ctx := context.Background()
	recommend(t, s, u[0], u[1])
	calls := 0
	fetch := func(context.Context, string) (media.Metadata, error) {
		calls++
		return media.Metadata{}, errors.New("offline")
	}
	for i := 0; i < 5; i++ {
		s.db.Exec("UPDATE metadata_jobs SET next_attempt=0")
		if err := s.Metadata(ctx, fetch); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 3 {
		t.Fatal("retry budget not enforced", calls)
	}
	if _, err := s.Recommend(ctx, u[0], u[1], "https://youtu.be/abcdefghijk", ""); err != nil {
		t.Fatal(err)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	if err := s.Metadata(cancelCtx, func(context.Context, string) (media.Metadata, error) {
		cancel()
		return media.Metadata{}, errors.New("cancelled")
	}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var attempts int
	s.db.QueryRow("SELECT attempts FROM metadata_jobs ORDER BY media_id DESC LIMIT 1").Scan(&attempts)
	if attempts != 0 {
		t.Fatal("shutdown spent a retry")
	}
}

func TestSessionsAndPasswordChangeAtomicity(t *testing.T) {
	s, u := fixture(t)
	ctx := context.Background()
	secret, err := testSession(t, s, u[0])
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	s.db.QueryRow("SELECT token_hash FROM sessions").Scan(&stored)
	if stored == secret || stored != token.Hash(secret) {
		t.Fatal("session stored in clear")
	}
	s.db.Exec("CREATE TRIGGER refuse_delete BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT,'injected failure'); END")
	_, before, _ := s.Credentials(ctx, "alice")
	if err := s.SetPassword(ctx, "alice", "another-long-password"); err == nil {
		t.Fatal("failure accepted")
	}
	_, after, _ := s.Credentials(ctx, "alice")
	if before != after {
		t.Fatal("password changed despite failed session revocation")
	}
	s.db.Exec("DROP TRIGGER refuse_delete")
	if err := s.SetPassword(ctx, "alice", "another-long-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Session(ctx, secret); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("old session survived", err)
	}
	secret, err = testSession(t, s, u[0])
	if err != nil {
		t.Fatal(err)
	}
	s.db.Exec("UPDATE sessions SET expires_at=?", time.Now().Unix()-1)
	if _, err := s.Session(ctx, secret); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("expired session accepted", err)
	}
}

func TestExportPrivacyAndSnapshot(t *testing.T) {
	s, u := fixture(t)
	ctx := context.Background()
	id := recommend(t, s, u[0], u[1])
	recommend(t, s, u[2], u[0])
	s.React(ctx, u[0], id, Reaction{"unheard", 0, "sender-secret-note"})
	s.React(ctx, u[1], id, Reaction{"listened", 1, "recipient-note"})
	s.AddComment(ctx, u[1], id, "my comment")
	archive, err := s.Export(ctx, User{ID: u[1], Username: "bobby"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.Recommendations) != 1 || len(archive.Comments) != 1 || strings.Contains(string(data), "sender-secret-note") || strings.Contains(string(data), "password") || strings.Contains(string(data), "token_hash") {
		t.Fatal("export privacy failed", string(data))
	}
	path := filepath.Join(t.TempDir(), "backup.db")
	if err := s.Backup(ctx, path); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshot(ctx, path); err != nil {
		t.Fatal(err)
	}
	if err := s.Backup(ctx, path); err == nil {
		t.Fatal("backup overwrote an existing file")
	}
	copy, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	if _, err := copy.Recommendation(ctx, u[1], id); err != nil {
		t.Fatal("snapshot lost data")
	}
}

func TestStaleCredentialsCannotCreateSession(t *testing.T) {
	s, u := fixture(t)
	ctx := context.Background()
	_, hash, err := s.Credentials(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPassword(ctx, "alice", "a-new-long-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NewSession(ctx, u[0], hash); !errors.Is(err, ErrMissing) {
		t.Fatal("stale credentials granted session", err)
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM sessions").Scan(&count); err != nil || count != 0 {
		t.Fatal("stale sign-in persisted", count, err)
	}
}

func TestConcurrentSubmissionsRemainDistinctAndComplete(t *testing.T) {
	s, u := fixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Recommend(ctx, u[0], u[1], "https://youtu.be/dQw4w9WgXcQ", "same link")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"media", "recommendations", "recommendation_destinations", "metadata_jobs"} {
		var count int
		if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != map[string]int{"media": 1, "recommendations": 10, "recommendation_destinations": 10, "metadata_jobs": 1}[table] {
			t.Fatal("concurrent submission incomplete", table, count, err)
		}
	}
}

func TestSnapshotSchemaValidation(t *testing.T) {
	ctx := context.Background()
	for _, table := range []string{"sessions", "comments", "metadata_jobs", "member_profiles"} {
		path := filepath.Join(t.TempDir(), "snapshot.db")
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec("DROP TABLE " + table); err != nil {
			t.Fatal(err)
		}
		s.Close()
		if err := ValidateSnapshot(ctx, path); err == nil {
			t.Fatal("incomplete schema accepted", table)
		}
	}
}

func testSession(t *testing.T, s *Store, user int64) (string, error) {
	t.Helper()
	var hash string
	if err := s.db.QueryRow("SELECT password_hash FROM users WHERE id=?", user).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	return s.NewSession(context.Background(), user, hash)
}
