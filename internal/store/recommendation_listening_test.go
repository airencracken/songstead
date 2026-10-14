// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"strconv"
	"testing"
	"testing/quick"
)

func TestPostingMarksAuthorListened(t *testing.T) {
	s, users := fixture(t)
	ctx := t.Context()
	group, err := s.SaveGroup(ctx, users[0], 0, "Music room", []int64{users[1]})
	if err != nil {
		t.Fatal(err)
	}
	labels := Labels{Genre: "Jazz", Tags: []string{"piano"}}
	posts := []struct {
		name string
		post func(string) (int64, error)
	}{
		{"legacy", func(raw string) (int64, error) { return s.Recommend(ctx, users[0], users[1], raw, "") }},
		{"person", func(raw string) (int64, error) {
			return s.RecommendMusic(ctx, users[0], users[1], 0, raw, "", "track", "Song", "")
		}},
		{"group", func(raw string) (int64, error) {
			return s.RecommendMusic(ctx, users[0], 0, group, raw, "", "album", "Album", "")
		}},
		{"members", func(raw string) (int64, error) { return s.ShareMusic(ctx, users[0], raw, "", "artist", "Artist", "") }},
		{"labeled person", func(raw string) (int64, error) {
			return s.RecommendLabeledMusic(ctx, users[0], users[1], 0, raw, "", "", "", "", labels)
		}},
		{"labeled members", func(raw string) (int64, error) {
			return s.ShareLabeledMusic(ctx, users[0], raw, "", "", "", "", labels)
		}},
	}
	for i, post := range posts {
		t.Run(post.name, func(t *testing.T) {
			id, err := post.post("https://music.example/post/" + strconv.Itoa(i))
			if err != nil {
				t.Fatal(err)
			}
			for i, viewer := range users[:2] {
				want := "unheard"
				if i == 0 {
					want = "listened"
				}
				item, err := s.Recommendation(ctx, viewer, id)
				if err != nil || item.Listening != want || item.Rating != 0 || item.PersonalNote != "" {
					t.Fatalf("viewer %d: want %s with no feedback, got %+v, %v", viewer, want, item, err)
				}
			}
		})
	}
}

func TestPostingListeningProperties(t *testing.T) {
	s, users := fixture(t)
	ctx := t.Context()
	id := recommend(t, s, users[0], users[1])
	statuses := []string{"unheard", "saved", "explored", "listened", "revisit", "dismissed"}
	property := func(authorState, recipientState, ratingSeed uint8) bool {
		rating := int(ratingSeed%3) - 1
		if err := s.React(ctx, users[0], id, Reaction{statuses[int(authorState)%len(statuses)], rating, "author note"}); err != nil {
			t.Error(err)
			return false
		}
		otherState := statuses[int(recipientState)%len(statuses)]
		if err := s.React(ctx, users[1], id, Reaction{otherState, -rating, "recipient note"}); err != nil {
			t.Error(err)
			return false
		}
		// Provider URL variants must update the same music state.
		posted, err := s.ShareMusic(ctx, users[0], "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "", "", "", "")
		if err != nil {
			t.Error(err)
			return false
		}
		author, err := s.Recommendation(ctx, users[0], id)
		if err != nil || author.Listening != "listened" || author.Rating != rating || author.PersonalNote != "author note" {
			return false
		}
		recipient, err := s.Recommendation(ctx, users[1], id)
		if err != nil || recipient.Listening != otherState || recipient.Rating != -rating || recipient.PersonalNote != "recipient note" {
			return false
		}
		fresh, err := s.Recommendation(ctx, users[0], posted)
		if err != nil || fresh.MediaID != author.MediaID || fresh.Listening != "listened" || fresh.Rating != 0 || fresh.PersonalNote != "" {
			return false
		}
		// Manual changes remain available after posting.
		return s.React(ctx, users[0], posted, Reaction{"revisit", 0, ""}) == nil
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 30}); err != nil {
		t.Fatal(err)
	}
}

func TestFailedPostingPreservesListening(t *testing.T) {
	s, users := fixture(t)
	ctx := t.Context()
	id := recommend(t, s, users[0], users[1])
	if err := s.React(ctx, users[0], id, Reaction{"saved", 1, "keep me"}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"music_states", "metadata_jobs"} {
		t.Run(table, func(t *testing.T) {
			if _, err := s.db.Exec("CREATE TRIGGER refuse_post BEFORE INSERT ON " + table + " BEGIN SELECT RAISE(ABORT,'injected failure'); END"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.ShareMusic(ctx, users[0], "https://youtu.be/dQw4w9WgXcQ", "", "", "", ""); err == nil {
				t.Fatal("failed post accepted")
			}
			item, err := s.Recommendation(ctx, users[0], id)
			if err != nil || item.Listening != "saved" || item.Rating != 1 || item.PersonalNote != "keep me" {
				t.Fatal("failed post changed existing feedback", item, err)
			}
			var count int
			if err := s.db.QueryRow("SELECT count(*) FROM recommendations").Scan(&count); err != nil || count != 1 {
				t.Fatal("partial recommendation survived", count, err)
			}
			if _, err := s.db.Exec("DROP TRIGGER refuse_post"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
