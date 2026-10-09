// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/quick"
	"time"
)

func TestDiscussionSettingsValidationLegacySchemaAndAtomicity(t *testing.T) {
	s, users, owner := adminFixture(t)
	ctx := t.Context()
	var seq int
	var name, path string
	if err := s.db.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"songstead", "witmoot", "both"} {
		v := DefaultSettings("https://music.example.org", "https://boards.example.org/forum")
		v.DiscussionMode = mode
		if err := s.SaveSettings(ctx, owner, &v); err != nil {
			t.Fatal(mode, err)
		}
		got, err := s.Settings(ctx, DefaultSettings("", ""))
		if err != nil || got != v {
			t.Fatal("mode did not persist", mode, got, err)
		}
		var stored string
		if err := s.db.QueryRow("SELECT json_extract(content,'$.DiscussionMode') FROM instance_settings").Scan(&stored); err != nil || stored != mode {
			t.Fatal("settings JSON contract", stored, err)
		}
	}
	before, _ := s.Settings(ctx, DefaultSettings("", ""))
	for _, mode := range []string{"", "local", "public", "BOTH", "<script>", "witmoot'; DROP TABLE users;--", strings.Repeat("x", 4096)} {
		bad := before
		bad.DiscussionMode = mode
		if err := s.SaveSettings(ctx, owner, &bad); !errors.Is(err, ErrInvalid) {
			t.Fatal("bad mode accepted", mode, err)
		}
		got, err := s.Settings(ctx, DefaultSettings("", ""))
		if err != nil || got != before {
			t.Fatal("invalid mode partially saved", got, err)
		}
	}
	for _, mode := range []string{"witmoot", "both"} {
		for _, field := range []string{"base", "witmoot"} {
			bad := before
			bad.DiscussionMode = mode
			if field == "base" {
				bad.BaseURL = ""
			} else {
				bad.WitmootURL = ""
			}
			if err := s.SaveSettings(ctx, owner, &bad); !errors.Is(err, ErrInvalid) {
				t.Fatal("missing connection accepted", mode, field, err)
			}
		}
	}
	if err := s.SaveSettings(ctx, users[0], &before); !errors.Is(err, ErrForbidden) {
		t.Fatal("member changed policy", err)
	}
	if _, err := s.db.Exec("CREATE TRIGGER fail_settings BEFORE UPDATE ON instance_settings BEGIN SELECT RAISE(ABORT,'failed policy write'); END"); err != nil {
		t.Fatal(err)
	}
	changed := before
	changed.DiscussionMode = "songstead"
	if err := s.SaveSettings(ctx, owner, &changed); err == nil {
		t.Fatal("failed policy write accepted")
	}
	got, err := s.Settings(ctx, DefaultSettings("", ""))
	if err != nil || got != before {
		t.Fatal("failed write changed policy", got, err)
	}
	if _, err := s.db.Exec("DROP TRIGGER fail_settings"); err != nil {
		t.Fatal(err)
	}
	// Older schema-7 settings and snapshots have no DiscussionMode field.
	for _, connected := range []bool{false, true} {
		v := DefaultSettings("", "")
		if connected {
			v.BaseURL, v.WitmootURL = "https://music.example.org", "https://boards.example.org"
		}
		data, _ := json.Marshal(v)
		var legacy map[string]any
		json.Unmarshal(data, &legacy)
		delete(legacy, "DiscussionMode")
		data, _ = json.Marshal(legacy)
		if _, err := s.db.Exec("UPDATE instance_settings SET content=?", string(data)); err != nil {
			t.Fatal(err)
		}
		got, err := s.Settings(ctx, DefaultSettings("", ""))
		want := "songstead"
		if connected {
			want = "both"
		}
		if err != nil || got.DiscussionMode != want || !got.LocalComments() || got.WitmootDiscussions() != connected {
			t.Fatal("legacy policy changed", got, err)
		}
		if err := ValidateSnapshot(ctx, path); err != nil {
			t.Fatal("legacy snapshot rejected", err)
		}
	}
	if err := quick.Check(func(value string) bool {
		v := DefaultSettings("https://music.example.org", "https://boards.example.org")
		v.DiscussionMode = value
		valid := value == "songstead" || value == "witmoot" || value == "both"
		return (ValidateSettings(v) == nil) == valid && v.LocalComments() == (value == "songstead" || value == "both") && v.WitmootDiscussions() == (value == "witmoot" || value == "both")
	}, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}

func TestDiscussionPolicyCommentAtomicityAndPrivacy(t *testing.T) {
	s, users, owner := adminFixture(t)
	ctx := t.Context()
	id := recommend(t, s, users[0], users[1])
	if err := s.AddComment(ctx, users[1], id, "Existing local comment"); err != nil {
		t.Fatal(err)
	}
	v := DefaultSettings("https://music.example.org", "https://boards.example.org")
	v.DiscussionMode = "witmoot"
	// A writer waiting for a committed settings change must observe the new
	// policy in the same transaction as its comment and timestamp writes.
	var seq int
	var name, path string
	if err := s.db.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	other, err := OpenCurrent(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	data, _ := json.Marshal(v)
	if _, err := tx.Exec("INSERT INTO instance_settings VALUES(1,?)", string(data)); err != nil {
		t.Fatal(err)
	}
	started, done := make(chan struct{}), make(chan error, 1)
	go func() { close(started); done <- other.Annotate(ctx, users[1], id, 0, "Should not persist at 0:30") }()
	<-started
	select {
	case err := <-done:
		t.Fatal("comment escaped policy transaction", err)
	case <-time.After(20 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrForbidden) {
		t.Fatal("disabled writer bypassed policy", err)
	}
	if err := s.AddComment(ctx, users[2], id, "Intruder"); !errors.Is(err, ErrMissing) {
		t.Fatal("policy revealed private recommendation", err)
	}
	list, err := s.Comments(ctx, users[1], id)
	if err != nil || len(list) != 1 || list[0].Body != "Existing local comment" {
		t.Fatal("old comments lost or rejected write persisted", list, err)
	}
	var offsets int
	s.db.QueryRow("SELECT count(*) FROM annotation_offsets").Scan(&offsets)
	if offsets != 0 {
		t.Fatal("rejected comment wrote timestamps", offsets)
	}
	if err := s.React(ctx, users[1], id, Reaction{"listened", 1, "Private even in Witmoot mode"}); err != nil {
		t.Fatal(err)
	}
	sender, err := s.Recommendation(ctx, users[0], id)
	if err != nil || sender.Rating != 0 || sender.PersonalNote != "" {
		t.Fatal("private feedback leaked", sender, err)
	}
	for _, mode := range []string{"songstead", "both"} {
		v.DiscussionMode = mode
		if err := s.SaveSettings(ctx, owner, &v); err != nil {
			t.Fatal(err)
		}
		if err := s.AddComment(ctx, users[1], id, "No Witmoot account needed"); err != nil {
			t.Fatal(mode, err)
		}
	}
}
