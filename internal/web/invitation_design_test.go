// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"bytes"
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/airencracken/songstead/internal/store"
)

func TestInvitationPageSchemaAndEscaping(t *testing.T) {
	a, _, owner := ownerBrowser(t)
	w := owner.request("GET", "/invites", nil)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	assertInvitationPageSchema(t, w.Body.String(), true)
	if !strings.Contains(w.Body.String(), "No invitations yet.") {
		t.Fatal("empty list has no explanation")
	}
	for _, label := range []string{"For Sam", strings.Repeat("w", 64), `<script>alert("invite")</script>`, `Quotes " & < >`} {
		w = owner.request("POST", "/invites", url.Values{"label": {label}, "max_uses": {"2"}, "days": {"7"}})
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		assertInvitationPageSchema(t, w.Body.String(), true)
		if !strings.Contains(w.Body.String(), "<h3>"+html.EscapeString(label)+"</h3>") || !strings.Contains(w.Body.String(), "/revoke") {
			t.Fatal("record lost its escaped label or revoke action")
		}
	}
	before, err := a.store.Invitations(t.Context(), stateUserID(t, a, "owner"), 0)
	if err != nil {
		t.Fatal(err)
	}
	w = owner.request("POST", "/invites", url.Values{"label": {"Keep my draft"}, "max_uses": {"-1"}, "days": {"7"}})
	if w.Code != 422 {
		t.Fatal(w.Code)
	}
	assertInvitationPageSchema(t, w.Body.String(), true)
	if !strings.Contains(w.Body.String(), `role="alert"`) || !strings.Contains(w.Body.String(), `value="Keep my draft"`) || !strings.Contains(w.Body.String(), `value="-1"`) {
		t.Fatal("validation lost the error or draft")
	}
	after, err := a.store.Invitations(t.Context(), stateUserID(t, a, "owner"), 0)
	if err != nil || len(before) != len(after) {
		t.Fatal("invalid form changed invitations", err)
	}
}

func TestInvitationStatusesDescribeAvailability(t *testing.T) {
	a, _, _ := ownerBrowser(t)
	for _, tc := range []struct {
		name       string
		invitation store.Invitation
		want       string
	}{
		{"open", store.Invitation{MaxUses: 1, ExpiresAt: 101}, "Open"},
		{"unlimited", store.Invitation{Uses: 20}, "Open"},
		{"used", store.Invitation{MaxUses: 1, Uses: 1}, "Used up"},
		{"expired", store.Invitation{ExpiresAt: 100}, "Expired"},
		{"revoked", store.Invitation{Revoked: true, ExpiresAt: 1, MaxUses: 1, Uses: 1}, "Revoked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			p := page{User: &store.User{Role: "owner"}, InvitationNow: 100, Invitations: []store.Invitation{tc.invitation}}
			if err := a.templates.ExecuteTemplate(&out, "invites", p); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), `<span class="invitation-status">`+tc.want+`</span>`) {
				t.Fatal("incorrect invitation status", out.String())
			}
		})
	}
}

func TestAccountPagesUseCompactForms(t *testing.T) {
	a, _, owner := ownerBrowser(t)
	w := owner.request("POST", "/invites", url.Values{"max_uses": {"1"}, "days": {"7"}})
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	link, err := url.Parse(secretFromPage(t, w.Body.String(), "join"))
	if err != nil {
		t.Fatal(err)
	}
	guest := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	for _, path := range []string{"/login", link.RequestURI()} {
		w = guest.request("GET", path, nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "account-page") || !strings.Contains(w.Body.String(), `autocomplete="username"`) {
			t.Fatalf("%s lost its compact account form", path)
		}
	}
}
