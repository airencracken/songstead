// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func activityRows(t *testing.T, s *Store) int {
	t.Helper()
	var rows int
	if err := s.db.QueryRow("SELECT count(*) FROM comfy_analytics_activity").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestSignInRecordsNoPerAccountActivity(t *testing.T) {
	s, users := fixture(t)
	for range 3 {
		_, hash, err := s.Credentials(context.Background(), "alice")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.NewSession(context.Background(), users[0], hash); err != nil {
			t.Fatal(err)
		}
	}
	if rows := activityRows(t, s); rows != 0 {
		t.Fatalf("sign-ins stored %d per-account rows", rows)
	}
	summary, err := s.Analytics(context.Background(), time.Now())
	if err != nil || summary.Logins != 0 || summary.ActiveMembers != 0 || len(summary.LoginsByAccount) != 0 || len(summary.Popular) != 0 {
		t.Fatal("per-account analytics survived", summary, err)
	}
}

func TestSchema12DeletesStoredActivityRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songstead.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	// Rows written by 0.15 and 0.16: sign-ins and per-item views, including
	// identifiers shaped like injection attempts.
	if _, err := s.db.Exec(`INSERT INTO comfy_analytics_activity(day,kind,member,item,count,last_at) VALUES
		(?, 'login', '1', '', 4, ?), (?, 'recommendation', '1', '7', 1, ?), (?, 'recommendation', '2', 'x'';--', 1, ?);
		PRAGMA user_version=11`, now/86400, now, now/86400, now, now/86400, now); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		s, err = Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var version int
		if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 12 {
			t.Fatal("schema", version, err)
		}
		if rows := activityRows(t, s); rows != 0 {
			t.Fatalf("migration kept %d activity rows", rows)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSchema12MigratesWithoutSharedActivityTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songstead.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("DROP TABLE comfy_analytics_activity; PRAGMA user_version=11"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal("migration needs the shared table", err)
	}
	defer s.Close()
	if rows := activityRows(t, s); rows != 0 {
		t.Fatal(rows)
	}
}

func TestSnapshotAcceptsSchema12(t *testing.T) {
	s, _ := fixture(t)
	target := filepath.Join(t.TempDir(), "backup.db")
	if err := s.Backup(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshot(context.Background(), target); err != nil {
		t.Fatal("schema 12 snapshot rejected", err)
	}
}
