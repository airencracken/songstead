// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"strings"
	"testing"
)

func section(t *testing.T, body, start, end string) string {
	t.Helper()
	from := strings.Index(body, start)
	if from < 0 {
		t.Fatalf("missing %s", start)
	}
	to := strings.Index(body[from:], end)
	if to < 0 {
		t.Fatalf("unterminated %s", start)
	}
	return body[from : from+to]
}

// Each destination has one home in the header, and the footer stays about the
// instance rather than repeating account actions.
func TestNavigationGivesEachDestinationOneHome(t *testing.T) {
	a, users, owner := ownerBrowser(t)
	if err := a.store.ChangeAccount(t.Context(), stateUserID(t, a, "owner"), users[0], "can_invite", "1"); err != nil {
		t.Fatal(err)
	}
	clients := map[string]*browser{"owner": owner, "delegate": login(t, a, "alice"), "member": login(t, a, "bobby")}
	for name, client := range clients {
		body := client.request("GET", "/shelf", nil).Body.String()
		header := section(t, body, `<header class="topbar">`, `</header>`)
		nav := strings.ReplaceAll(header, `class="brand" href="/shelf"`, "")
		for _, href := range []string{`href="/shelf"`, `href="/recent"`, `href="/history"`, `href="/search"`, `href="/recommendations/new"`, `href="/account"`, `href="/groups"`} {
			if count := strings.Count(nav, href); count != 1 {
				t.Errorf("%s: %s appears %d times", name, href, count)
			}
		}
		invites := strings.Count(header, `href="/invites"`) + strings.Count(header, `href="/admin/invites"`)
		if want := map[string]int{"owner": 0, "delegate": 1, "member": 0}[name]; invites != want {
			t.Errorf("%s: %d invitation links in the header, want %d", name, invites, want)
		}
		if (name == "owner") != strings.Contains(header, `href="/admin/settings"`) {
			t.Errorf("%s: wrong Admin link", name)
		}
		if strings.Count(header, `data-theme-select`) != 1 {
			t.Errorf("%s: theme picker repeated", name)
		}
		if footer := section(t, body, `<footer`, `</footer>`); strings.Contains(footer, "/account/export") {
			t.Errorf("%s: footer repeats the export link", name)
		}
		if !strings.Contains(client.request("GET", "/account", nil).Body.String(), `href="/account/export"`) {
			t.Errorf("%s: settings lost the export link", name)
		}
	}
	settings := owner.request("GET", "/admin/settings", nil).Body.String()
	adminNav := section(t, settings, `<nav class="admin-nav"`, `</nav>`)
	if !strings.Contains(adminNav, `href="/invites"`) || strings.Contains(adminNav, "/admin/invites") {
		t.Error("admin navigation should link to the one invitations page")
	}
}
