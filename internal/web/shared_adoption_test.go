// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/airencracken/comfylib/autobackup"
	"github.com/airencracken/comfylib/brandimage"
	"github.com/airencracken/comfylib/ui"
	"github.com/airencracken/songstead/internal/store"
)

func TestNewPasswordsGetPlainReasons(t *testing.T) {
	for _, tc := range []struct{ secret, confirmation, want string }{
		{"a-long-test-password", "a-long-test-password", ""},
		{"a-long-test-password", "a-different-password", "The two passwords don't match."},
		{"short", "short", "Use at least 12 characters."},
		{strings.Repeat("é", 40), strings.Repeat("é", 40), "Use a shorter password."},
		{"invalid\xffpassword-text", "invalid\xffpassword-text", "That password has characters that can't be saved. Try typing it again."},
	} {
		if got := newPasswordProblem(tc.secret, tc.confirmation); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.secret, got, tc.want)
		}
	}
}

func TestPasswordFormsExplainTheRule(t *testing.T) {
	a, _ := fixture(t)
	alice := login(t, a, "alice")
	long := strings.Repeat("é", 40) // 40 characters, 80 bytes
	w := alice.request("POST", "/account/password", url.Values{"current_password": {"a-long-test-password"}, "password": {long}, "confirmation": {long}})
	if w.Code != 422 || !strings.Contains(w.Body.String(), "Use a shorter password.") || strings.Contains(w.Body.String(), "bytes") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = alice.request("POST", "/account/password", url.Values{"current_password": {"wrong-current-password"}, "password": {"another-long-password"}, "confirmation": {"another-long-password"}})
	if w.Code != 422 || !strings.Contains(w.Body.String(), "current password didn&#39;t match") {
		t.Fatal(w.Code, w.Body.String())
	}
	if body := alice.request("GET", "/account", nil).Body.String(); strings.Contains(body, "72") && strings.Contains(body, "bytes") {
		t.Fatal("password help mentions bytes")
	}
}

// suite.js moves a copy input into its own row, so each one needs an explicit
// label, not a wrapping one.
func TestOneTimeLinksGetCopyButtons(t *testing.T) {
	_, users, owner := ownerBrowser(t)
	copyInput := regexp.MustCompile(`<input id="([a-z-]+)"[^>]*data-comfy-copy`)
	w := owner.request("POST", "/invites", url.Values{"max_uses": {"1"}, "days": {"7"}})
	reset := owner.request("POST", fmt.Sprintf("/admin/users/%d/reset", users[1]), nil)
	for name, body := range map[string]string{"invitation": w.Body.String(), "recovery": reset.Body.String()} {
		match := copyInput.FindStringSubmatch(body)
		if match == nil || !strings.Contains(body, `<label for="`+match[1]+`">`) {
			t.Fatalf("%s link lacks a labelled copy input", name)
		}
	}
}

func TestBackupFailuresShowOnlyTheSummary(t *testing.T) {
	a, _ := fixture(t)
	var out bytes.Buffer
	p := page{User: &store.User{Role: "owner"}, Backups: autobackup.Status{Enabled: true, LastError: "open /var/lib/songstead/backups: permission denied"}}
	if err := a.templates.ExecuteTemplate(&out, "UTC", "settings", p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), autobackup.FailureSummary) || strings.Contains(out.String(), "permission denied") {
		t.Fatal("raw backup error reached the page")
	}
}

// Upload limits are binary values, and the copy names them in IEC units from
// the same constant, so the two cannot drift apart.
func TestUploadLimitCopyMatchesTheLimit(t *testing.T) {
	if brandimage.MaxBytes != 2<<20 {
		t.Fatalf("image limit is %d bytes, want 2 MiB", brandimage.MaxBytes)
	}
	if got := ui.Size(brandimage.MaxBytes); got != "2 MiB" {
		t.Fatalf("limit formats as %q", got)
	}
	_, _, owner := ownerBrowser(t)
	for _, path := range []string{"/admin/settings", "/account"} {
		body := owner.request("GET", path, nil).Body.String()
		if !strings.Contains(body, "up to 2 MiB and") || regexp.MustCompile(`\d MB\b`).MatchString(body) {
			t.Fatalf("%s: upload limit copy disagrees with the limit", path)
		}
	}
	for _, doc := range []string{"../../docs/administration.md", "../../docs/quiet-inbox.md"} {
		data, err := os.ReadFile(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "up to 2 MiB and") || regexp.MustCompile(`\d(\.\d)? MB\b`).Match(data) {
			t.Fatalf("%s: upload limit copy disagrees with the limit", doc)
		}
	}
}
