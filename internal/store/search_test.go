// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestSearchAudienceLiteralUnicodeAndReadOnly(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	public, err := s.ShareLabeledMusic(ctx, u[0], "https://music.example/shared", "Message about a moon", "track", "ÉCHO 100%_", "Jóhann", Labels{Genre: "Ambient", Tags: []string{"Late night", "日本語"}})
	if err != nil {
		t.Fatal(err)
	}
	private, err := s.RecommendLabeledMusic(ctx, u[0], u[1], 0, "https://music.example/gift", "private needle", "track", "Private title", "", Labels{Genre: "HiddenGenre"})
	if err != nil {
		t.Fatal(err)
	}
	group, err := s.SaveGroup(ctx, u[0], 0, "Circle", []int64{u[1]})
	if err != nil {
		t.Fatal(err)
	}
	grouped, err := s.RecommendMusic(ctx, u[0], 0, group, "https://music.example/group", "group needle", "track", "Group title", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{public, private, grouped} {
		if err := s.AddComment(ctx, u[0], id, "needle reaction"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.React(ctx, u[1], public, Reaction{"listened", -1, "SECRET-RATING-NOTE"}); err != nil {
		t.Fatal(err)
	}
	before, err := s.Export(ctx, User{ID: u[1], Username: "bobby"})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"écho", "JÓHANN", "ambient", "NIGHT", "日本語", "moon", "100%_"} {
		got, err := s.SearchMusic(ctx, u[2], q, 0, 25)
		if err != nil || len(got) != 1 || got[0].ID != public {
			t.Fatal("field or Unicode search", q, got, err)
		}
	}
	for _, q := range []string{"%", "_"} {
		got, err := s.SearchMusic(ctx, u[2], q, 0, 25)
		if err != nil || len(got) != 1 {
			t.Fatal("wildcard interpreted", got, err)
		}
	}
	for _, q := range []string{"SECRET-RATING-NOTE", "private needle", "HiddenGenre", "group needle", "' OR 1=1 --"} {
		got, err := s.SearchMusic(ctx, u[2], q, 0, 25)
		if err != nil || len(got) != 0 {
			t.Fatal("private data or SQL input", q, got, err)
		}
	}
	for i, viewer := range u {
		music, err := s.SearchMusic(ctx, viewer, "title", 0, 25)
		want := 2
		if i == 2 {
			want = 0
		}
		if err != nil || len(music) != want {
			t.Fatal("music audience", i, music, err)
		}
		comments, err := s.SearchComments(ctx, viewer, "needle", 0, 25)
		want = 3
		if i == 2 {
			want = 1
		}
		if err != nil || len(comments) != want {
			t.Fatal("comment audience", i, comments, err)
		}
	}
	after, err := s.Export(ctx, User{ID: u[1], Username: "bobby"})
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("search mutated export", err)
	}
	for _, id := range []int64{999999, u[2]} {
		if id == u[2] {
			if _, err := s.db.Exec("UPDATE users SET suspended=1 WHERE id=?", id); err != nil {
				t.Fatal(err)
			}
		}
		music, err := s.SearchMusic(ctx, id, "écho", 0, 25)
		if err != nil || len(music) != 0 {
			t.Fatal("inactive music", music, err)
		}
		comments, err := s.SearchComments(ctx, id, "needle", 0, 25)
		if err != nil || len(comments) != 0 {
			t.Fatal("inactive comments", comments, err)
		}
	}
	for _, raw := range []string{strings.Repeat("界", 201), "a\x00b", "a\nb", string([]byte{255})} {
		if _, err := SearchQuery(raw); !errors.Is(err, ErrInvalid) {
			t.Fatal("bad query", raw, err)
		}
	}
	for _, args := range []struct {
		viewer, before int64
		limit          int
	}{{0, 0, 25}, {u[0], -1, 25}, {u[0], 0, 0}, {u[0], 0, 101}} {
		if _, err := s.SearchMusic(ctx, args.viewer, "x", args.before, args.limit); !errors.Is(err, ErrInvalid) {
			t.Fatal("music bounds", err)
		}
		if _, err := s.SearchComments(ctx, args.viewer, "x", args.before, args.limit); !errors.Is(err, ErrInvalid) {
			t.Fatal("comment bounds", err)
		}
	}
}

func TestSearchSpoilerPropertiesAndCursorPagination(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	id := sharedCommentMusic(t, s, u[0])
	for _, body := range []string{"needle plain", "needle early 0:10", "needle several 0:10 and 0:40", "needle late 1:00"} {
		if err := s.AddComment(ctx, u[0], id, body); err != nil {
			t.Fatal(err)
		}
	}
	tracks, err := s.Recordings(ctx, u[1], id)
	if err != nil || len(tracks) != 1 {
		t.Fatal(err)
	}
	comments, err := s.Comments(ctx, u[1], id)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"immediate", "spoiler-free", "hidden"} {
		if err := s.SetAnnotationMode(ctx, u[1], mode); err != nil {
			t.Fatal(err)
		}
		for position := 0; position <= 65; position++ {
			if err := s.SetPosition(ctx, u[1], id, tracks[0].ID, position); err != nil {
				t.Fatal(err)
			}
			tracks[0].Position = position
			want := VisibleComments(comments, tracks, mode, false)
			got, err := s.SearchComments(ctx, u[1], "needle", 0, 100)
			if err != nil || len(got) != len(want) {
				t.Fatal("spoiler property", mode, position, got, err)
			}
		}
	}
	for i := 0; i < 27; i++ {
		if err := s.AddComment(ctx, u[0], id, fmt.Sprintf("needle page %02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 30; i++ {
		if err := s.AddComment(ctx, u[0], id, "needle hidden 1:00"); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.SearchComments(ctx, u[1], "needle", 0, 25)
	if err != nil || len(first) != 25 || first[0].Body != "needle page 26" {
		t.Fatal("hidden comments affected page", first, err)
	}
	cursor := first[24].ID
	if err := s.AddComment(ctx, u[0], id, "needle arrived"); err != nil {
		t.Fatal(err)
	}
	second, err := s.SearchComments(ctx, u[1], "needle", cursor, 25)
	if err != nil || len(second) != 3 || second[0].Body != "needle page 01" {
		t.Fatal("comment cursor", second, err)
	}
	for i := 0; i < 28; i++ {
		if _, err := s.ShareMusic(ctx, u[0], fmt.Sprintf("https://music.example/%d", i), "", "track", fmt.Sprintf("Needle song %02d", i), ""); err != nil {
			t.Fatal(err)
		}
	}
	music, err := s.SearchMusic(ctx, u[1], "needle", 0, 25)
	if err != nil || len(music) != 25 || music[0].Title != "Needle song 27" {
		t.Fatal("music ordering", music, err)
	}
	if _, err := s.ShareMusic(ctx, u[0], "https://music.example/new", "", "track", "Needle new", ""); err != nil {
		t.Fatal(err)
	}
	older, err := s.SearchMusic(ctx, u[1], "needle", music[24].ID, 25)
	if err != nil || len(older) != 3 || older[0].Title != "Needle song 02" {
		t.Fatal("music cursor", older, err)
	}
}
