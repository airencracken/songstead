// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"net/http"
	"strings"
	"testing"
)

func htmlTag(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, "<html")
	if start < 0 {
		t.Fatal("no html element")
	}
	return body[start : start+strings.Index(body[start:], ">")]
}

func TestOwnerPicksTheSitePalette(t *testing.T) {
	a, _, owner := ownerBrowser(t)
	guest := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	tag := htmlTag(t, guest.request("GET", "/login", nil).Body.String())
	for _, want := range []string{`data-palette="sage"`, `data-palette-default="sage"`, `data-palettes="sage slate"`, `data-theme-key="songstead-theme"`} {
		if !strings.Contains(tag, want) {
			t.Fatalf("default page lacks %s: %s", want, tag)
		}
	}
	values := settingsValues()
	values.Set("palette", "slate")
	if w := owner.request("POST", "/admin/settings", values); w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	if tag := htmlTag(t, guest.request("GET", "/login", nil).Body.String()); !strings.Contains(tag, `data-palette="slate"`) {
		t.Fatal("saved palette not applied", tag)
	}
	// Older forms without the field keep the saved palette.
	older := settingsValues()
	if w := owner.request("POST", "/admin/settings", older); w.Code != 303 {
		t.Fatal(w.Code)
	}
	if tag := htmlTag(t, guest.request("GET", "/login", nil).Body.String()); !strings.Contains(tag, `data-palette="slate"`) {
		t.Fatal("older form dropped the palette", tag)
	}
	for _, bad := range []string{"neon", "Slate", `slate" onload="x`, "<script>", strings.Repeat("s", 200)} {
		values.Set("palette", bad)
		if w := owner.request("POST", "/admin/settings", values); w.Code != 422 {
			t.Fatalf("palette %q accepted: %d", bad, w.Code)
		}
	}
	if tag := htmlTag(t, guest.request("GET", "/login", nil).Body.String()); !strings.Contains(tag, `data-palette="slate"`) || strings.Contains(tag, "onload") {
		t.Fatal("rejected palette changed the page", tag)
	}
	member := login(t, a, "alice")
	values.Set("palette", "sage")
	if w := member.request("POST", "/admin/settings", values); w.Code != 403 {
		t.Fatal("member changed the palette", w.Code)
	}
}

func TestPagesUseTheSharedThemeScriptAndPicker(t *testing.T) {
	a, _ := fixture(t)
	body := login(t, a, "alice").request("GET", "/shelf", nil).Body.String()
	head := body[:strings.Index(body, "</head>")]
	if !strings.Contains(head, `<script src="/static/comfyware/theme.js"></script>`) || strings.Contains(head, "/static/theme.js") {
		t.Fatal("theme.js must be the shared script, loaded without defer")
	}
	if strings.Index(head, "/static/app.css") > strings.Index(head, "/static/comfyware/palettes.css") {
		t.Fatal("palettes.css must follow the app stylesheet")
	}
	if !strings.Contains(body, `data-palette-select`) || strings.Count(body, `data-theme-select`) != 1 {
		t.Fatal("missing the shared theme and palette picker")
	}
}
