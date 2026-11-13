// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func assertProblemPage(t *testing.T, w *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	body := w.Body.String()
	if w.Code != status {
		t.Fatalf("status %d, want %d: %s", w.Code, status, body)
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(body, `class="panel problem"`) || !strings.Contains(body, `<header class="topbar">`) {
		t.Fatal("bare error page", body)
	}
	if !strings.Contains(body, message) {
		t.Fatalf("missing %q: %s", message, body)
	}
	for _, leak := range []string{"permission denied", "invalid input", "form too large", "duplicate form fields", "MaxBytes", "sql:", "SONGSTEAD_"} {
		if strings.Contains(body, leak) {
			t.Fatalf("internal detail %q reached the page", leak)
		}
	}
}

func TestProblemsUseTheStyledPage(t *testing.T) {
	a, _, _ := ownerBrowser(t)
	alice := login(t, a, "alice")
	assertProblemPage(t, alice.request("GET", "/admin/settings", nil), 403, "Your account can&#39;t open this page.")
	assertProblemPage(t, alice.request("GET", "/admin/invites", nil), 403, "Your account can&#39;t open this page.")
	assertProblemPage(t, alice.request("GET", "/recommendations/999999", nil), 404, "isn&#39;t shared with you")
	assertProblemPage(t, alice.request("GET", "/members/999999", nil), 404, "isn&#39;t shared with you")
	for _, path := range []string{"/shelf?layout=grid", "/recent?discovery=%00", "/history?offset=-1", "/shelf?liked=2", "/shelf?person=-4", "/shelf?kind=%3Cscript%3E", "/shelf?view=everything"} {
		assertProblemPage(t, alice.request("GET", path, nil), 400, "That view isn&#39;t available.")
	}
}

func TestFormProblemsUseTheStyledPage(t *testing.T) {
	a, _ := fixture(t)
	alice := login(t, a, "alice")
	htmx := false
	send := func(body, contentType string, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/recommendations/new", strings.NewReader(body))
		if htmx {
			r.Header.Set("HX-Request", "true")
		}
		r.RemoteAddr = "192.0.2.1:1234"
		r.Header.Set("Content-Type", contentType)
		if csrf {
			r.Header.Set("X-CSRF-Token", alice.csrf)
		}
		for _, c := range alice.cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w
	}
	form := "application/x-www-form-urlencoded"
	assertProblemPage(t, send("note="+strings.Repeat("a", 20000), form, true), 413, "too large to save")
	assertProblemPage(t, send("url=a&url=b", form, true), 400, "That form couldn&#39;t be read.")
	assertProblemPage(t, send("url=%zz", form, true), 400, "That form couldn&#39;t be read.")
	htmx = true
	w := send("url=https%3A%2F%2Fexample.org", form, false)
	htmx = false
	assertProblemPage(t, w, 403, "Reload it and try again.")
	if w.Header().Get("HX-Refresh") != "true" {
		t.Fatal("expired forms no longer ask htmx to reload")
	}
	if w := send("url=https%3A%2F%2Fexample.org", form, false); w.Header().Get("HX-Refresh") != "" {
		t.Fatal("plain form posts should not ask for an htmx reload")
	}
	guest := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	guest.request("GET", "/login", nil)
	var last *httptest.ResponseRecorder
	for range 25 {
		last = guest.request("POST", "/login", url.Values{"username": {"alice"}, "password": {"wrong-password-attempt"}})
	}
	assertProblemPage(t, last, 429, "Too many sign-in attempts.")
}

func TestServerErrorsKeepDetailOutOfThePage(t *testing.T) {
	a, _ := fixture(t)
	alice := login(t, a, "alice")
	if _, err := a.store.DB().Exec("DROP TABLE recommendation_labels"); err != nil {
		t.Fatal(err)
	}
	w := alice.request("GET", "/shelf", nil)
	assertProblemPage(t, w, 500, "Something went wrong")
	if strings.Contains(w.Body.String(), "recommendation_labels") || strings.Contains(w.Body.String(), "no such table") {
		t.Fatal("database detail reached the page")
	}
}

func TestProblemsAnswerAPIClientsWithJSON(t *testing.T) {
	a, _ := fixture(t)
	alice := login(t, a, "alice")
	r := httptest.NewRequest("GET", "/recommendations/999999", nil)
	r.Header.Set("Accept", "application/json")
	for _, c := range alice.cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	if w.Code != 404 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || !strings.Contains(w.Body.String(), `"status":404`) {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
}
