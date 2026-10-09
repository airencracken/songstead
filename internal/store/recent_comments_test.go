// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func sharedCommentMusic(t *testing.T, s *Store, user int64) int64 {
	t.Helper()
	id, err := s.ShareMusic(t.Context(), user, "https://music.example/song", "A recommendation note", "track", "Porchlight song", "Demo artist")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRecentCommentsAudienceProjectionAndPagination(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	shared := sharedCommentMusic(t, s, u[0])
	second := sharedCommentMusic(t, s, u[2])
	private := recommend(t, s, u[0], u[1])
	group, err := s.SaveGroup(ctx, u[0], 0, "Secret group", []int64{u[1]})
	if err != nil {
		t.Fatal(err)
	}
	groupGift, err := s.RecommendMusic(ctx, u[0], 0, group, "https://music.example/song", "Secret group note", "track", "Porchlight song", "Demo artist")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.React(ctx, u[1], shared, Reaction{"listened", -1, "SECRET personal notes"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 53; i++ {
		id := shared
		if i%2 == 1 {
			id = second
		}
		if err := s.AddComment(ctx, u[1], id, fmt.Sprintf("Shared reaction %02d", i)); err != nil {
			t.Fatal(err)
		}
		if err := s.AddComment(ctx, u[0], private, "SECRET direct comment"); err != nil {
			t.Fatal(err)
		}
		if err := s.AddComment(ctx, u[1], groupGift, "SECRET group comment"); err != nil {
			t.Fatal(err)
		}
	}
	for _, viewer := range u {
		rows, err := s.RecentComments(ctx, viewer, 0, 100)
		if err != nil || len(rows) != 53 {
			t.Fatal("audience projection", viewer, len(rows), err)
		}
		for i, row := range rows {
			if row.Body != fmt.Sprintf("Shared reaction %02d", 52-i) || row.AuthorID != u[1] || row.Title != "Porchlight song" || row.Artist != "Demo artist" || strings.Contains(fmt.Sprint(row), "SECRET") {
				t.Fatal("private context or ordering", row)
			}
		}
	}
	first, err := s.RecentComments(ctx, u[2], 0, 25)
	if err != nil {
		t.Fatal(err)
	}
	cursor := first[len(first)-1].ID
	if err := s.AddComment(ctx, u[0], shared, "Arrived between pages"); err != nil {
		t.Fatal(err)
	}
	older, err := s.RecentComments(ctx, u[2], cursor, 25)
	if err != nil || len(older) != 25 || older[0].Body != "Shared reaction 27" {
		t.Fatal("unstable older page", older, err)
	}
	end, err := s.RecentComments(ctx, u[2], older[len(older)-1].ID, 25)
	if err != nil || len(end) != 3 || end[2].Body != "Shared reaction 00" {
		t.Fatal("wrong final page", end, err)
	}
	for _, args := range []struct {
		viewer, before int64
		limit          int
	}{{0, 0, 25}, {-1, 0, 25}, {u[0], -1, 25}, {u[0], 0, 0}, {u[0], 0, 101}} {
		if _, err := s.RecentComments(ctx, args.viewer, args.before, args.limit); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid bounds accepted", args, err)
		}
	}
	if rows, err := s.RecentComments(ctx, 999999, 0, 25); err != nil || len(rows) != 0 {
		t.Fatal("non-member read", rows, err)
	}
	if _, err := s.db.Exec("UPDATE users SET suspended=1 WHERE id=?", u[2]); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.RecentComments(ctx, u[2], 0, 25); err != nil || len(rows) != 0 {
		t.Fatal("suspended member read", rows, err)
	}
}

func TestRecentCommentsSpoilerPropertiesAndPageBoundaries(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	shared := sharedCommentMusic(t, s, u[0])
	if err := s.AddComment(ctx, u[0], shared, "Plain conversation"); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"Early at 0:10", "Several moments 0:10 and 0:40", "Late at 1:00"} {
		if err := s.AddComment(ctx, u[0], shared, body); err != nil {
			t.Fatal(err)
		}
	}
	tracks, err := s.Recordings(ctx, u[1], shared)
	if err != nil || len(tracks) != 1 {
		t.Fatal(err, tracks)
	}
	comments, err := s.Comments(ctx, u[1], shared)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"immediate", "spoiler-free", "hidden"} {
		if err := s.SetAnnotationMode(ctx, u[1], mode); err != nil {
			t.Fatal(err)
		}
		for position := 0; position <= 65; position++ {
			if err := s.SetPosition(ctx, u[1], shared, tracks[0].ID, position); err != nil {
				t.Fatal(err)
			}
			tracks[0].Position = position
			want := VisibleComments(comments, tracks, mode, false)
			got, err := s.RecentComments(ctx, u[1], 0, 100)
			if err != nil || len(got) != len(want) {
				t.Fatal("spoiler policy mismatch", mode, position, len(got), len(want), err)
			}
			for i, c := range got {
				if c.ID != want[len(want)-1-i].ID {
					t.Fatal("spoiler projection", mode, position, c)
				}
			}
			page, err := s.RecentComments(ctx, u[1], 0, 1)
			if err != nil || len(page) != 1 || page[0].ID != got[0].ID {
				t.Fatal("concealed comment consumed page", mode, position, page, err)
			}
		}
	}
	// Positions belong to each viewer and apply to the recording across contexts.
	if err := s.SetAnnotationMode(ctx, u[2], "spoiler-free"); err != nil {
		t.Fatal(err)
	}
	got, err := s.RecentComments(ctx, u[2], 0, 25)
	if err != nil || len(got) != 1 || got[0].Body != "Plain conversation" {
		t.Fatal("someone else's position exposed annotations", got, err)
	}
	// Even hidden mode retains comments without timestamp annotations.
	if err := s.SetAnnotationMode(ctx, u[2], "hidden"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		if err := s.AddComment(ctx, u[0], shared, "Concealed at 0:20"); err != nil {
			t.Fatal(err)
		}
	}
	got, err = s.RecentComments(ctx, u[2], 0, 1)
	if err != nil || len(got) != 1 || got[0].Body != "Plain conversation" {
		t.Fatal("hidden comments starved page", got, err)
	}
}

func TestRecentCommentsSchemaReadOnlyAndCommentAtomicity(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	shared := sharedCommentMusic(t, s, u[0])
	if err := s.AddComment(ctx, u[0], shared, "Kept comment"); err != nil {
		t.Fatal(err)
	}
	before, err := s.RecentComments(ctx, u[1], 0, 25)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("CREATE TRIGGER reject_feed_offset BEFORE INSERT ON annotation_offsets BEGIN SELECT RAISE(ABORT,'test offset failure'); END;"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddComment(ctx, u[0], shared, "Partial comment at 0:10"); err == nil {
		t.Fatal("failed transaction succeeded")
	}
	after, err := s.RecentComments(ctx, u[1], 0, 25)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("partial annotation appeared in feed", after, err)
	}
	var version, count int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 8 {
		t.Fatal("feed changed schema", version, err)
	}
	if err := s.db.QueryRow("SELECT count(*) FROM comments").Scan(&count); err != nil || count != 1 {
		t.Fatal("feed read or failed write changed comments", count, err)
	}
	settings := DefaultSettings("https://songs.example", "https://boards.example")
	settings.DiscussionMode = "witmoot"
	if err := s.SetRole(ctx, "alice", "owner"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSettings(ctx, u[0], &settings); err != nil {
		t.Fatal(err)
	}
	after, err = s.RecentComments(ctx, u[1], 0, 25)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("Witmoot-only setting hid existing comments", after, err)
	}
}
