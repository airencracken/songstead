// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/airencracken/comfylib/autobackup"
)

func TestAutomaticBackupRestoresPrivateStateAndRejectsCap(t *testing.T) {
	s, u := fixture(t)
	id := recommend(t, s, u[0], u[1])
	want := Reaction{"listened", -1, "private listening note"}
	if err := s.React(t.Context(), u[1], id, want); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "snapshot")
	if err := s.AutomaticSnapshot(t.Context(), dest, 10<<20); err != nil {
		t.Fatal(err)
	}
	restored, err := OpenCurrent(filepath.Join(dest, "songstead.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	got, err := restored.Recommendation(t.Context(), u[1], id)
	if err != nil || got.PersonalNote != want.Note || got.Rating != -1 {
		t.Fatal(got, err)
	}
	if _, err := restored.Recommendation(t.Context(), u[2], id); err == nil {
		t.Fatal("restored private recommendation became visible")
	}
	for _, user := range u {
		left := User{ID: user}
		var err error
		if err != nil {
			t.Fatal(err)
		}
		before, err := s.Export(t.Context(), left)
		if err != nil {
			t.Fatal(err)
		}
		after, err := restored.Export(t.Context(), left)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, after) {
			a, _ := json.Marshal(before)
			b, _ := json.Marshal(after)
			t.Fatal(string(a), string(b))
		}
	}
	dest = filepath.Join(t.TempDir(), "too-small")
	if err := s.AutomaticSnapshot(t.Context(), dest, 1); err == nil {
		t.Fatal("copy cap ignored")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("oversized database copied before cap check", err)
	}
	if err := autobackup.CheckDatabase(t.Context(), s.db, -1); err == nil {
		t.Fatal("negative limit accepted")
	}
}

func TestAutomaticBackupRejectsInvalidForeignKeys(t *testing.T) {
	s, u := fixture(t)
	recommend(t, s, u[0], u[1])
	if _, err := s.db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("DELETE FROM users WHERE id=?", u[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.AutomaticSnapshot(t.Context(), filepath.Join(t.TempDir(), "invalid"), 10<<20); err == nil {
		t.Fatal("invalid snapshot accepted")
	}
}
