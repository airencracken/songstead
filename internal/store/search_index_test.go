// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSearchIndexTracksMutationsAndRollback(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	id, e := s.ShareLabeledMusic(ctx, u[0], "https://music.example/index", "original-note", "track", "Écho of rain", "One band", Labels{Genre: "Ambient", Tags: []string{"blue evening"}})
	if e != nil {
		t.Fatal(e)
	}
	check := func(query string, want bool) {
		t.Helper()
		rows, e := s.SearchMusic(ctx, u[1], query, 0, 30)
		if e != nil || (len(rows) == 1) != want {
			t.Fatal(query, rows, e, want)
		}
	}
	check("ÉCHO", true)
	check(`" OR *`, false)
	check("blue evening", true)
	for _, step := range []struct{ sql, old, new string }{
		{"UPDATE recommendations SET note='revised-note',title_override='Another title' WHERE id=?", "original-note", "revised-note"},
		{"UPDATE recommendation_labels SET tags='[\"green morning\"]',genre='Chamber' WHERE recommendation_id=?", "blue evening", "green morning"},
		{"DELETE FROM recommendation_labels WHERE recommendation_id=?", "green morning", "revised-note"},
	} {
		if _, e = s.db.Exec(step.sql, id); e != nil {
			t.Fatal(e)
		}
		check(step.old, false)
		check(step.new, true)
	}
	tx, e := s.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec("UPDATE recommendations SET note='rolled-back' WHERE id=?", id); e != nil {
		t.Fatal(e)
	}
	if e = tx.Rollback(); e != nil {
		t.Fatal(e)
	}
	check("rolled-back", false)
	check("revised-note", true)
	if _, e = s.db.Exec("UPDATE media SET artist='Changed artist' WHERE id=(SELECT media_id FROM recommendations WHERE id=?)", id); e != nil {
		t.Fatal(e)
	}
	check("Changed artist", true)
	check("One band", false)
	if _, e = s.db.Exec("DELETE FROM recommendations WHERE id=?", id); e != nil {
		t.Fatal(e)
	}
	check("revised-note", false)
	var n int
	if e = s.db.QueryRow("SELECT count(*) FROM music_search WHERE rowid=?", id).Scan(&n); e != nil || n != 0 {
		t.Fatal("stale deleted index", n, e)
	}
}
func TestSearchIndexCandidatesAndScanAgree(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	for i := 0; i < 270; i++ {
		_, e := s.ShareLabeledMusic(ctx, u[0], fmt.Sprintf("https://music.example/%d", i), "Shared needle", "track", fmt.Sprintf("Title %d", i), "Band", Labels{Tags: []string{"élan", `quote \" here`}})
		if e != nil {
			t.Fatal(e)
		}
	}
	ids := func(rows []Recommendation) []int64 {
		out := []int64{}
		for _, r := range rows {
			out = append(out, r.ID)
		}
		return out
	}
	for _, q := range []string{"needle", "Title 12", "élan", "a", "%", "' OR 1=1 --", `quote \"`, "missing"} {
		indexed, e := s.SearchMusic(ctx, u[1], q, 250, 30)
		if e != nil {
			t.Fatal(e)
		}
		// Disable the candidate index in a transaction only while calculating an
		// independent literal predicate over the authorized reference rows.
		rows, e := s.db.QueryContext(ctx, selectRecommendation+` WHERE `+visible+` AND r.id<250 ORDER BY r.id DESC`, u[1], u[1], u[1], u[1], u[1])
		if e != nil {
			t.Fatal(e)
		}
		want := []Recommendation{}
		for rows.Next() {
			r, e := scanRecommendation(rows)
			if e != nil {
				t.Fatal(e)
			}
			fields := append([]string{r.Title, r.Artist, r.Note, r.Genre}, r.Tags...)
			for _, field := range fields {
				if strings.Contains(strings.ToLower(field), strings.ToLower(q)) {
					want = append(want, r)
					break
				}
			}
			if len(want) == 30 {
				break
			}
		}
		rows.Close()
		if !reflect.DeepEqual(ids(indexed), ids(want)) {
			t.Fatal(q, ids(indexed), ids(want))
		}
	}
}

func TestSearchIndexMigrationFailureAtomicityAndSnapshotValidation(t *testing.T) {
	s, u := fixture(t)
	id, e := s.ShareMusic(t.Context(), u[0], "https://music.example/keep", "keep this note", "track", "Keep title", "")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec(`DROP TRIGGER search_recommendations_insert;DROP TRIGGER search_recommendations_update;DROP TRIGGER search_recommendations_delete;DROP TRIGGER search_recommendation_labels_insert;DROP TRIGGER search_recommendation_labels_update;DROP TRIGGER search_recommendation_labels_delete;DROP TRIGGER search_media_update;DROP TABLE music_search;CREATE TABLE music_search(block_migration);PRAGMA user_version=10`); e != nil {
		t.Fatal(e)
	}
	if e = s.migrate(false); e == nil {
		t.Fatal("migration ignored conflicting schema")
	}
	var version, triggers int
	if e = s.db.QueryRow("PRAGMA user_version").Scan(&version); e != nil || version != 10 {
		t.Fatal("failed migration changed version", version, e)
	}
	if e = s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='trigger' AND name LIKE 'search_%'").Scan(&triggers); e != nil || triggers != 0 {
		t.Fatal("partial search triggers", triggers, e)
	}
	if _, e = s.db.Exec("DROP TABLE music_search"); e != nil {
		t.Fatal(e)
	}
	if e = s.migrate(false); e != nil {
		t.Fatal(e)
	}
	rows, e := s.SearchMusic(t.Context(), u[1], "Keep title", 0, 30)
	if e != nil || len(rows) != 1 || rows[0].ID != id {
		t.Fatal("migration lost existing search data", rows, e)
	}
	path := filepath.Join(t.TempDir(), "snapshot.db")
	if e = s.Backup(t.Context(), path); e != nil {
		t.Fatal(e)
	}
	if e = ValidateSnapshot(t.Context(), path); e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec("DROP TRIGGER search_media_update"); e != nil {
		t.Fatal(e)
	}
	broken := filepath.Join(t.TempDir(), "incomplete.db")
	if e = s.Backup(t.Context(), broken); e != nil {
		t.Fatal(e)
	}
	if e = ValidateSnapshot(t.Context(), broken); e == nil {
		t.Fatal("snapshot accepted incomplete search schema")
	}
}
