// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/airencracken/comfylib/analytics/analyticstest"
)

func detailTableExists(t *testing.T, s *Store) bool {
	t.Helper()
	var tables int
	if err := s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='comfy_analytics_activity'").Scan(&tables); err != nil {
		t.Fatal(err)
	}
	return tables > 0
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
	if detailTableExists(t, s) {
		t.Fatal("the per-account activity table still exists")
	}
	summary, err := s.Analytics(context.Background(), time.Now())
	if err != nil || summary.Requests != 0 || summary.FailedSignIns != 0 || len(summary.LoginsByAccount) != 0 || len(summary.Popular) != 0 {
		t.Fatal("sign-ins were counted", summary, err)
	}
}

func TestSchema12RemovesStoredActivity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songstead.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	// The table and rows 0.15 and 0.16 kept, including an identifier shaped
	// like an injection attempt.
	if _, err := s.db.Exec(`CREATE TABLE comfy_analytics_activity(day INTEGER, kind TEXT, member TEXT, item TEXT, count INTEGER, last_at INTEGER, PRIMARY KEY(day,kind,member,item));
		INSERT INTO comfy_analytics_activity VALUES(?, 'login', '1', '', 4, ?), (?, 'recommendation', '2', 'x'';--', 1, ?);
		PRAGMA user_version=11`, now/86400, now, now/86400, now); err != nil {
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
		if detailTableExists(t, s) {
			t.Fatal("migration kept per-account activity")
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSchema12MigrationRemovesDetailInsideTheMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songstead.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("CREATE TABLE comfy_analytics_activity(x); PRAGMA user_version=11"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// Run only the application migrations, without the shared Initialize
	// step, so the migration itself must remove the table.
	s = &Store{}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	s.db = db
	defer db.Close()
	if err := s.migrate(false); err != nil {
		t.Fatal(err)
	}
	if detailTableExists(t, s) {
		t.Fatal("schema 12 left the per-account table to the shared initializer")
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

func TestSharedMetricsStorageWithThisDriver(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "metrics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	analyticstest.Run(t, db)
}
