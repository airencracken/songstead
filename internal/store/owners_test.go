// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/quick"

	"golang.org/x/crypto/bcrypt"
)

func TestOwnerProvisioningAndDuplicateAtomicity(t *testing.T) {
	s, users := fixture(t)
	ctx := t.Context()
	member, hash, err := s.Credentials(ctx, "alice")
	if err != nil || member.Role != "member" {
		t.Fatal("member role", member, err)
	}
	session, err := s.NewSession(ctx, users[0], hash)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alice", "ALICE"} {
		if _, err := s.CreateOwner(ctx, name, "a-different-password"); err == nil {
			t.Fatal("owner command changed an existing member")
		}
		u, currentHash, err := s.Credentials(ctx, name)
		if err != nil || u != member || currentHash != hash {
			t.Fatal("duplicate owner changed credentials", u, err)
		}
		if _, err := s.Session(ctx, session); err != nil {
			t.Fatal("failed provisioning revoked a session", err)
		}
	}
	id, err := s.CreateOwner(ctx, "alex", "a-long-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	owner, ownerHash, err := s.Credentials(ctx, "alex")
	if err != nil || owner.ID != id || owner.Role != "owner" || bcrypt.CompareHashAndPassword([]byte(ownerHash), []byte("a-long-owner-password")) != nil {
		t.Fatal("owner credentials", owner, err)
	}
	ownerSession, err := s.NewSession(ctx, id, ownerHash)
	if err != nil {
		t.Fatal(err)
	}
	if u, err := s.Session(ctx, ownerSession); err != nil || u.Role != "owner" {
		t.Fatal("session lost owner role", u, err)
	}
	if err := s.SetPassword(ctx, "alex", "another-owner-password"); err != nil {
		t.Fatal(err)
	}
	if u, _, err := s.Credentials(ctx, "alex"); err != nil || u.Role != "owner" {
		t.Fatal("password reset changed owner role", u, err)
	}
	all, err := s.Users(ctx, 0)
	if err != nil || len(all) != 4 || all[0].Role != "owner" {
		t.Fatal("account listing lost role", all, err)
	}
}

func TestConcurrentOwnerProvisioning(t *testing.T) {
	s, _ := fixture(t)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, name := range []string{"alex", "ALEX"} {
		wg.Go(func() { _, err := s.CreateOwner(t.Context(), name, "a-long-owner-password"); results <- err })
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM users WHERE role='owner'").Scan(&count); err != nil || count != 1 || success != 1 {
		t.Fatal("concurrent provisioning was not atomic", count, success, err)
	}
}

func TestOwnerSchemaAndAdversarialInputs(t *testing.T) {
	s, _ := fixture(t)
	for _, role := range []any{"admin", "OWNER", "", "owner'); DROP TABLE users;--", nil} {
		if _, err := s.db.Exec("UPDATE users SET role=? WHERE username='alice'", role); err == nil {
			t.Fatal("schema accepted invalid role", role)
		}
	}
	for _, name := range []string{"ab", "a b", "alex'; DROP TABLE users;--", "\xff", strings.Repeat("a", 25)} {
		if _, err := s.CreateOwner(t.Context(), name, "a-long-owner-password"); err == nil {
			t.Fatal("invalid owner username accepted", name)
		}
	}
	for _, password := range []string{"short", strings.Repeat("x", 73), "\xff-long-password"} {
		if _, err := s.CreateOwner(t.Context(), "alex", password); err == nil {
			t.Fatal("invalid owner password accepted")
		}
	}
	property := func(raw string) bool {
		err := ValidateUsername(raw)
		if err != nil {
			return true
		}
		return len(raw) >= 3 && len(raw) <= 24 && strings.IndexFunc(raw, func(r rune) bool {
			return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-')
		}) < 0
	}
	if err := quick.Check(property, nil); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerMigrationAndLegacySnapshots(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "songstead.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(ctx, "alice", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	before, hash, err := s.Credentials(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := s.NewSession(ctx, before.ID, hash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("DROP TABLE media_artwork; DROP TABLE recommendation_labels; DROP TABLE discovery_preferences; DROP TABLE branding_assets; DROP TABLE password_resets; DROP TABLE invitations; DROP TABLE instance_settings; ALTER TABLE users DROP COLUMN invited_by; ALTER TABLE users DROP COLUMN can_invite; ALTER TABLE users DROP COLUMN suspended; ALTER TABLE users DROP COLUMN role; PRAGMA user_version=3;"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := ValidateSnapshot(ctx, path); err != nil {
		t.Fatal("previous release backup rejected", err)
	}
	if _, err := OpenCurrent(path); err == nil || !strings.Contains(err.Error(), "restart") {
		t.Fatal("account command migrated the live old schema", err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	after, currentHash, err := s.Credentials(ctx, "alice")
	if err != nil || after != before || currentHash != hash {
		t.Fatal("owner migration changed existing credentials", after, err)
	}
	if user, err := s.Session(ctx, secret); err != nil || user != before {
		t.Fatal("owner migration changed sessions", user, err)
	}
	if err := ValidateSnapshot(ctx, path); err != nil {
		t.Fatal("current snapshot rejected", err)
	}
	if _, err := s.db.Exec("PRAGMA ignore_check_constraints=ON; UPDATE users SET role='admin';"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshot(ctx, path); err == nil {
		t.Fatal("forged role snapshot accepted")
	}
}

func TestOwnerMigrationFailureIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songstead.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOwner(t.Context(), "alex", "a-long-owner-password"); err != nil {
		t.Fatal(err)
	}
	// A conflicting column causes migration 4 to fail rather than overwrite it.
	if _, err := s.db.Exec("PRAGMA user_version=3"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := Open(path); err == nil {
		t.Fatal("conflicting migration accepted")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	var role string
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 3 {
		t.Fatal("failed migration changed schema version", version, err)
	}
	if err := db.QueryRow("SELECT role FROM users WHERE username='alex'").Scan(&role); err != nil || role != "owner" {
		t.Fatal("failed migration changed account", role, err)
	}
}
