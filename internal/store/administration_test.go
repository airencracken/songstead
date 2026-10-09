// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"bytes"
	"database/sql"
	"errors"
	"github.com/airencracken/comfylib/token"
	"golang.org/x/crypto/bcrypt"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/quick"
)

func adminFixture(t *testing.T) (*Store, []int64, int64) {
	t.Helper()
	s, users := fixture(t)
	owner, err := s.CreateOwner(t.Context(), "owner", "a-long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	return s, users, owner
}
func TestSettingsPersistenceDefaultsAndPermissions(t *testing.T) {
	s, u, owner := adminFixture(t)
	ctx := t.Context()
	defaults := DefaultSettings("https://music.example.org", "https://boards.example.org")
	v, err := s.Settings(ctx, defaults)
	if err != nil || v != defaults {
		t.Fatal(v, err)
	}
	v.Name = "Friends"
	v.WitmootURL = ""
	v.DiscussionMode = "songstead"
	v.JoinMode = "closed"
	v.HouseRules = "Be kind."
	v.ShowVersion = true
	if err := s.SaveSettings(ctx, u[0], &v); !errors.Is(err, ErrForbidden) {
		t.Fatal("member saved settings", err)
	}
	if err := s.SaveSettings(ctx, owner, &v); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Settings(ctx, defaults); err != nil || got != v || got.WitmootURL != "" {
		t.Fatal("blank override not persisted", got, err)
	}
	if _, err := s.CreateInvitation(ctx, owner, "", 1, 7); !errors.Is(err, ErrInvalid) {
		t.Fatal("created invite when closed", err)
	}
	if err := s.SaveSettings(ctx, owner, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Settings(ctx, defaults); err != nil || got != defaults {
		t.Fatal("defaults not restored", got, err)
	}
	for _, raw := range []string{"javascript:alert(1)", "https://u:p@example.org", "https://example.org/?token=private", "https://example.org/#private", "https://example.org:65536", "https://example.org/subpath"} {
		bad := defaults
		bad.BaseURL = raw
		if ValidateSettings(bad) == nil {
			t.Fatal("invalid public address accepted", raw)
		}
	}
	for _, mode := range []string{"", "public", "invite'; DROP TABLE users;--"} {
		bad := defaults
		bad.JoinMode = mode
		if ValidateSettings(bad) == nil {
			t.Fatal("invalid policy accepted")
		}
	}
}
func TestInvitationLifecycleAndAtomicJoin(t *testing.T) {
	s, u, owner := adminFixture(t)
	ctx := t.Context()
	if _, err := s.CreateInvitation(ctx, u[0], "Alice", 1, 7); !errors.Is(err, ErrForbidden) {
		t.Fatal("member invited", err)
	}
	if err := s.ChangeAccount(ctx, owner, u[0], "can_invite", "1"); err != nil {
		t.Fatal(err)
	}
	raw, err := s.CreateInvitation(ctx, u[0], "For a friend", 1, 7)
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := s.db.QueryRow("SELECT token_hash FROM invitations").Scan(&stored); err != nil || stored == raw || stored != token.Hash(raw) {
		t.Fatal("raw token stored", err)
	}
	for i := 0; i < 2; i++ {
		if err := s.CheckInvitation(ctx, raw); err != nil {
			t.Fatal("GET consumed invite", err)
		}
	}
	if _, err := s.Join(ctx, raw, "ALICE", "a-long-test-password"); !errors.Is(err, ErrInvalid) {
		t.Fatal("duplicate username accepted", err)
	}
	if err := s.CheckInvitation(ctx, raw); err != nil {
		t.Fatal("failed join consumed invite", err)
	}
	if _, err := s.db.Exec("CREATE TRIGGER fail_invite BEFORE UPDATE OF uses ON invitations BEGIN SELECT RAISE(ABORT,'failed consume'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Join(ctx, raw, "guest", "a-long-test-password"); err == nil {
		t.Fatal("partial registration accepted")
	}
	if _, _, err := s.Credentials(ctx, "guest"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("user leaked after failed invite consumption", err)
	}
	if _, err := s.db.Exec("DROP TRIGGER fail_invite"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, name := range []string{"guest1", "guest2"} {
		wg.Go(func() { _, err := s.Join(ctx, raw, name, "a-long-test-password"); results <- err })
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("invite used more than once", success)
	}
	if err := s.CheckInvitation(ctx, raw); !errors.Is(err, ErrInvalid) {
		t.Fatal("exhausted invite valid", err)
	}
	accounts, err := s.Accounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range accounts {
		if strings.HasPrefix(a.Username, "guest") && (a.Role != "member" || a.InvitedBy != "alice" || a.CanInvite) {
			t.Fatal("registration privileges/attribution", a)
		}
	}
	other, err := s.CreateInvitation(ctx, owner, "owner only", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := s.Invitations(ctx, u[0], 0)
	if err != nil || len(listed) != 1 {
		t.Fatal("delegate sees others' invites", listed, err)
	}
	if err := s.RevokeInvitation(ctx, u[0], 2); !errors.Is(err, ErrMissing) {
		t.Fatal("delegate revoked owner invite", err)
	}
	if err := s.RevokeInvitation(ctx, owner, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckInvitation(ctx, other); !errors.Is(err, ErrInvalid) {
		t.Fatal("revoked invite valid", err)
	}
	raw, err = s.CreateInvitation(ctx, u[0], "delegate", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeAccount(ctx, owner, u[0], "can_invite", "0"); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeAccount(ctx, owner, u[0], "can_invite", "1"); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckInvitation(ctx, raw); !errors.Is(err, ErrInvalid) {
		t.Fatal("permission regrant revived invitation", err)
	}
}
func TestInvitationExpiryPolicyAndAdversarialInputs(t *testing.T) {
	s, _, owner := adminFixture(t)
	ctx := t.Context()
	raw, err := s.CreateInvitation(ctx, owner, "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE invitations SET expires_at=1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Join(ctx, raw, "guest", "a-long-test-password"); !errors.Is(err, ErrInvalid) {
		t.Fatal("expired invite accepted", err)
	}
	for _, bad := range []string{"", "abc", strings.Repeat("a", 64), "sgi_'+DROP TABLE users;--", "sgr_aaaaaaaaaaaa_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		if bad != "" && s.CheckInvitation(ctx, bad) == nil {
			t.Fatal("wrong-purpose invite accepted")
		}
	}
	for _, v := range []struct {
		uses, days int
		label      string
	}{{-1, 1, ""}, {10001, 1, ""}, {1, -1, ""}, {1, 3651, ""}, {1, 7, strings.Repeat("x", 65)}, {1, 7, "\x00"}} {
		if _, err := s.CreateInvitation(ctx, owner, v.label, v.uses, v.days); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid invitation options", err)
		}
	}
	settings := DefaultSettings("", "")
	settings.JoinMode = "open"
	if err := s.SaveSettings(ctx, owner, &settings); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Join(ctx, "", "guest", "a-long-test-password"); err != nil {
		t.Fatal("explicit open signup failed", err)
	}
	if _, err := s.Join(ctx, "bad-token", "guest2", "a-long-test-password"); !errors.Is(err, ErrInvalid) {
		t.Fatal("open mode bypassed invalid token", err)
	}
	settings.JoinMode = "closed"
	if err := s.SaveSettings(ctx, owner, &settings); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Join(ctx, "", "guest2", "a-long-test-password"); !errors.Is(err, ErrInvalid) {
		t.Fatal("closed joining accepted", err)
	}
}
func TestLastOwnerSuspensionAndPrivilegeAtomicity(t *testing.T) {
	s, u, owner := adminFixture(t)
	ctx := t.Context()
	for _, v := range []struct{ action, value string }{{"role", "member"}, {"suspended", "1"}} {
		if err := s.ChangeAccount(ctx, owner, owner, v.action, v.value); !errors.Is(err, ErrInvalid) {
			t.Fatal("last owner removed", err)
		}
	}
	if err := s.SetRole(ctx, "owner", "member"); !errors.Is(err, ErrInvalid) {
		t.Fatal("CLI removed last owner", err)
	}
	if err := s.ChangeAccount(ctx, u[0], u[0], "role", "owner"); !errors.Is(err, ErrForbidden) {
		t.Fatal("self-promotion allowed", err)
	}
	if err := s.ChangeAccount(ctx, owner, u[0], "can_invite", "1"); err != nil {
		t.Fatal(err)
	}
	raw, err := s.CreateInvitation(ctx, u[0], "", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	reset, err := s.CreatePasswordReset(ctx, owner, u[0])
	if err != nil {
		t.Fatal(err)
	}
	_, hash, err := s.Credentials(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := s.NewSession(ctx, u[0], hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeAccount(ctx, owner, u[0], "suspended", "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Session(ctx, secret); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("suspended session accepted", err)
	}
	if _, err := s.NewSession(ctx, u[0], hash); !errors.Is(err, ErrMissing) {
		t.Fatal("suspended login accepted", err)
	}
	if s.CheckInvitation(ctx, raw) == nil || s.CheckPasswordReset(ctx, reset) == nil {
		t.Fatal("suspension left bearer links valid")
	}
	if err := s.ChangeAccount(ctx, owner, u[0], "suspended", "0"); err != nil {
		t.Fatal(err)
	}
	if s.CheckInvitation(ctx, raw) == nil {
		t.Fatal("resume revived old invitation")
	}
	if err := s.SetRole(ctx, "alice", "owner"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, id := range []int64{owner, u[0]} {
		wg.Go(func() { results <- s.ChangeAccount(ctx, id, id, "role", "member") })
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM users WHERE role='owner' AND suspended=0").Scan(&count); err != nil || count != 1 || successes != 1 {
		t.Fatal("concurrent demotions removed all owners", count, successes, err)
	}
}
func TestRecoverySingleUseExpiryAndAtomicity(t *testing.T) {
	s, u, owner := adminFixture(t)
	ctx := t.Context()
	if _, err := s.CreatePasswordReset(ctx, u[0], u[1]); !errors.Is(err, ErrForbidden) {
		t.Fatal("member reset another account", err)
	}
	_, oldHash, err := s.Credentials(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.NewSession(ctx, u[0], oldHash)
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.CreatePasswordReset(ctx, owner, u[0])
	if err != nil {
		t.Fatal(err)
	}
	raw, err := s.CreatePasswordReset(ctx, owner, u[0])
	if err != nil {
		t.Fatal(err)
	}
	if s.CheckPasswordReset(ctx, old) == nil {
		t.Fatal("replacement did not revoke old reset")
	}
	for i := 0; i < 2; i++ {
		if err := s.CheckPasswordReset(ctx, raw); err != nil {
			t.Fatal("GET consumed reset", err)
		}
	}
	if _, err := s.db.Exec("CREATE TRIGGER fail_reset BEFORE DELETE ON password_resets BEGIN SELECT RAISE(ABORT,'cannot consume reset'); END"); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, raw, "a-new-test-password"); err == nil {
		t.Fatal("failed reset accepted")
	}
	if _, hash, err := s.Credentials(ctx, "alice"); err != nil || hash != oldHash {
		t.Fatal("partial password change", err)
	}
	if _, err := s.Session(ctx, session); err != nil {
		t.Fatal("failed reset revoked session", err)
	}
	if _, err := s.db.Exec("DROP TRIGGER fail_reset"); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, raw, "a-new-test-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, raw, "another-new-password"); !errors.Is(err, ErrInvalid) {
		t.Fatal("reset replay accepted", err)
	}
	if _, err := s.Session(ctx, session); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("reset left old session valid", err)
	}
	if _, hash, err := s.Credentials(ctx, "alice"); err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte("a-new-test-password")) != nil {
		t.Fatal("password not replaced", err)
	}
	raw, err = s.CreatePasswordReset(ctx, owner, u[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE password_resets SET expires_at=1"); err != nil {
		t.Fatal(err)
	}
	if s.CheckPasswordReset(ctx, raw) == nil {
		t.Fatal("expired reset accepted")
	}
	raw, err = s.CreatePasswordReset(ctx, owner, u[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPassword(ctx, "alice", "a-local-new-password"); err != nil {
		t.Fatal(err)
	}
	if s.CheckPasswordReset(ctx, raw) == nil {
		t.Fatal("local password change kept old reset")
	}
	if err := s.ChangePassword(ctx, u[0], oldHash, "an-old-race-password"); !errors.Is(err, ErrInvalid) {
		t.Fatal("stale credential changed password", err)
	}
	invite, err := s.CreateInvitation(ctx, owner, "", 1, 7)
	if err != nil {
		t.Fatal(err)
	}
	if s.CheckPasswordReset(ctx, invite) == nil {
		t.Fatal("invite accepted as reset")
	}
}
func TestAdministrationSchemaMigrationAndSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songstead.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := s.CreateOwner(t.Context(), "owner", "a-long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	_, hash, _ := s.Credentials(t.Context(), "owner")
	session, err := s.NewSession(t.Context(), owner, hash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("DROP TABLE member_profiles; DROP TABLE user_profiles; DROP TABLE media_artwork; DROP TABLE recommendation_labels; DROP TABLE discovery_preferences; DROP TABLE branding_assets; DROP TABLE password_resets; DROP TABLE invitations; DROP TABLE instance_settings; ALTER TABLE users DROP COLUMN invited_by; ALTER TABLE users DROP COLUMN can_invite; ALTER TABLE users DROP COLUMN suspended; PRAGMA user_version=4;"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := ValidateSnapshot(t.Context(), path); err != nil {
		t.Fatal("old snapshot rejected", err)
	}
	if _, err := OpenCurrent(path); err == nil {
		t.Fatal("live command migrated old database")
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if u, err := s.Session(t.Context(), session); err != nil || u.Role != "owner" || u.CanInvite || u.Suspended {
		t.Fatal("migration changed account/session", u, err)
	}
	if err := ValidateSnapshot(t.Context(), path); err != nil {
		t.Fatal("new snapshot rejected", err)
	}
	for _, query := range []string{"UPDATE users SET suspended=2", "UPDATE users SET can_invite=-1", "INSERT INTO instance_settings VALUES(2,'{}')", "INSERT INTO instance_settings VALUES(1,'bad-json')", "INSERT INTO branding_assets VALUES('script',X'01')"} {
		if _, err := s.db.Exec(query); err == nil {
			t.Fatal("schema accepted invalid data", query)
		}
	}
	if _, err := s.CreateInvitation(t.Context(), owner, "", 1, 7); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"UPDATE invitations SET max_uses=-1", "UPDATE invitations SET uses=2", "UPDATE invitations SET revoked=2", "UPDATE invitations SET token_hash='raw'"} {
		if _, err := s.db.Exec(query); err == nil {
			t.Fatal("schema accepted invalid invite", query)
		}
	}
}
func TestSettingsAdversarialProperty(t *testing.T) {
	property := func(raw string) bool {
		v := DefaultSettings("", "")
		v.Name = raw
		err := ValidateSettings(v)
		if err != nil {
			return true
		}
		return validText(raw, 1, 80) && !strings.ContainsAny(raw, "\n\r\t")
	}
	if err := quick.Check(property, nil); err != nil {
		t.Fatal(err)
	}
}
func TestBrandingAtomicityAndPrivacy(t *testing.T) {
	s, u, owner := adminFixture(t)
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewGray(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	img := out.Bytes()
	if err := s.SaveBrandingAssets(t.Context(), u[0], img, nil, false, false); !errors.Is(err, ErrForbidden) {
		t.Fatal("member saved images", err)
	}
	if err := s.SaveBrandingAssets(t.Context(), owner, img, []byte("bad"), false, false); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid image accepted", err)
	}
	if _, err := s.BrandingAsset(t.Context(), "mascot"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("partial image save", err)
	}
	if _, err := s.db.Exec("CREATE TRIGGER fail_brand BEFORE INSERT ON branding_assets WHEN NEW.name='favicon' BEGIN SELECT RAISE(ABORT,'no favicon'); END"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveBrandingAssets(t.Context(), owner, img, img, false, false); err == nil {
		t.Fatal("failed save accepted")
	}
	if _, err := s.BrandingAsset(t.Context(), "mascot"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("partial transactional image save", err)
	}
	if _, err := s.db.Exec("DROP TRIGGER fail_brand"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveBrandingAssets(t.Context(), owner, img, img, false, false); err != nil {
		t.Fatal(err)
	}
	id, err := s.Recommend(t.Context(), u[0], u[1], "https://music.example.org/secret", "private provenance")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Recommendation(t.Context(), owner, id); !errors.Is(err, ErrMissing) {
		t.Fatal("owner gained private recommendation access", err)
	}
	if err := s.AddComment(t.Context(), owner, id, "private intrusion"); !errors.Is(err, ErrMissing) {
		t.Fatal("owner gained private comment access", err)
	}
}

func TestSuspendedStaleSessionRejected(t *testing.T) {
	s, u, _ := adminFixture(t)
	_, hash, err := s.Credentials(t.Context(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := s.NewSession(t.Context(), u[0], hash)
	if err != nil {
		t.Fatal(err)
	}
	// A valid legacy or restored session row cannot override current suspension.
	if _, err := s.db.Exec("UPDATE users SET suspended=1 WHERE id=?", u[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Session(t.Context(), secret); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("stale suspended session accepted", err)
	}
	if _, err := s.NewSession(t.Context(), u[0], hash); !errors.Is(err, ErrMissing) {
		t.Fatal("suspended account minted session", err)
	}
}

func TestConcurrentLastOwnerAcrossDatabaseHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songstead.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	alex, err := first.CreateOwner(t.Context(), "alex", "a-long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	freya, err := first.CreateOwner(t.Context(), "freya", "a-long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() { <-start; results <- first.ChangeAccount(t.Context(), alex, alex, "role", "member") }()
	go func() { <-start; results <- second.ChangeAccount(t.Context(), freya, freya, "suspended", "1") }()
	close(start)
	success := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			success++
		}
	}
	var count int
	if err := first.db.QueryRow("SELECT count(*) FROM users WHERE role='owner' AND suspended=0").Scan(&count); err != nil || count != 1 || success != 1 {
		t.Fatal("concurrent CLI/server handles removed last owner", count, success, err)
	}
}
