// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Metrics describe site health. Requests are counted per route pattern;
// sign-ins and opens never store who signed in or what they read.
func TestMetricsCountRoutesAndSecurityWithoutPeople(t *testing.T) {
	a, users, owner := ownerBrowser(t)
	alice := login(t, a, "alice")
	id, err := a.store.Recommend(t.Context(), users[0], users[1], "https://example.org/song", "private activity fixture")
	if err != nil {
		t.Fatal(err)
	}
	alice.request("GET", fmt.Sprintf("/recommendations/%d?secret=query", id), nil)
	guest := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	guest.request("GET", "/login", nil)
	guest.request("POST", "/login", url.Values{"username": {"alice"}, "password": {"not-the-password"}})
	guest.request("POST", "/login", url.Values{"username": {"nobody"}, "password": {strings.Repeat("é", 40)}})
	guest.request("GET", "/join?invite=forged-invitation", nil)
	guest.request("GET", "/reset?token=forged-reset", nil)
	if err := a.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	summary, err := a.store.Analytics(t.Context(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Requests == 0 || summary.FailedSignIns != 2 || summary.RejectedTokens != 2 {
		t.Fatal("site health counts", summary.Traffic, summary.Security)
	}
	var routes []string
	rows, err := a.store.DB().Query("SELECT route FROM comfy_analytics_routes")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var route string
		if err := rows.Scan(&route); err != nil {
			t.Fatal(err)
		}
		routes = append(routes, route)
	}
	rows.Close()
	joined := strings.Join(routes, "\n")
	if !strings.Contains(joined, "GET /recommendations/{id}") || strings.Contains(joined, fmt.Sprint(id)) || strings.Contains(joined, "secret") || strings.Contains(joined, "forged") {
		t.Fatal("route labels carry paths or queries", joined)
	}
	body := owner.request("GET", "/admin/metrics", nil).Body.String()
	if !strings.Contains(body, "Instance overview") || strings.Contains(body, "private activity fixture") || strings.Contains(body, ">alice<") {
		t.Fatal("dashboard leaks people or content", body)
	}
	if w := alice.request("GET", "/admin/metrics?role=owner", nil); w.Code != 403 || strings.Contains(w.Body.String(), "Instance overview") {
		t.Fatal("member read metrics", w.Code)
	}
}

func TestRefusedAttemptsAreCounted(t *testing.T) {
	a, _ := fixture(t)
	guest := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	guest.request("GET", "/login", nil)
	for range 25 {
		guest.request("POST", "/login", url.Values{"username": {"alice"}, "password": {"wrong-password-attempt"}})
	}
	summary, err := a.store.Analytics(t.Context(), time.Now())
	if err != nil || summary.RateLimited != 5 || summary.FailedSignIns != 20 {
		t.Fatal("refused attempts", summary.Security, err)
	}
}
