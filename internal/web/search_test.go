// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestSearchRoutesPrivacySpoilersAndPagination(t *testing.T) {
	a, u := fixture(t)
	ctx := t.Context()
	b := login(t, a, "carol")
	for i := 0; i < 27; i++ {
		if _, err := a.store.ShareMusic(ctx, u[0], fmt.Sprintf("https://music.example/%d", i), "note", "track", fmt.Sprintf("Needle & <song> %02d", i), "Artist"); err != nil {
			t.Fatal(err)
		}
	}
	id, err := a.store.RecommendMusic(ctx, u[0], u[1], 0, "https://music.example/private", "SECRET gift", "track", "Needle private", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.AddComment(ctx, u[0], id, "Needle private comment"); err != nil {
		t.Fatal(err)
	}
	shared, err := a.store.ShareMusic(ctx, u[0], "https://music.example/shared", "", "track", "Public conversation", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.AddComment(ctx, u[0], shared, "Needle <script>visible</script>"); err != nil {
		t.Fatal(err)
	}
	if err := a.store.AddComment(ctx, u[0], shared, "Needle SECRET spoiler 1:00"); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetAnnotationMode(ctx, u[2], "hidden"); err != nil {
		t.Fatal(err)
	}
	w := b.request("GET", "/search?q=needle", nil)
	body := w.Body.String()
	if w.Code != 200 || strings.Contains(body, "SECRET") || strings.Contains(body, "Needle private") || strings.Count(body, `class="panel recommendation-copy"`) != 25 || !strings.Contains(body, "&lt;song&gt;") || !strings.Contains(body, `href="/search" aria-current="page"`) {
		t.Fatal("search UI or privacy", w.Code, body)
	}
	next := regexp.MustCompile(`href="(/search\?q=needle&amp;kind=music&amp;before=[0-9]+)"`).FindStringSubmatch(body)
	if len(next) != 2 {
		t.Fatal("missing cursor link", body)
	}
	w = b.request("GET", strings.ReplaceAll(next[1], "&amp;", "&"), nil)
	if w.Code != 200 || strings.Count(w.Body.String(), `class="panel recommendation-copy"`) != 2 || !strings.Contains(w.Body.String(), "Newest matches") {
		t.Fatal("older results", w.Code, w.Body.String())
	}
	w = b.request("GET", "/search?kind=comments&q=needle&reveal=1", nil)
	body = w.Body.String()
	if w.Code != 200 || strings.Contains(body, "SECRET") || strings.Contains(body, "private comment") || !strings.Contains(body, "&lt;script&gt;visible&lt;/script&gt;") || !strings.Contains(body, fmt.Sprintf("/recommendations/%d#comment-", shared)) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("comment search contract", w.Code, body)
	}
	for _, raw := range []string{"q=a&q=b", "kind=x", "kind=music&kind=comments", "before=0", "before=-1", "before=1&before=2", "before=x", "before=9223372036854775808", "q=%ZZ", "q=a%00b", "q=" + url.QueryEscape(strings.Repeat("界", 201))} {
		if w := b.request("GET", "/search?"+raw, nil); w.Code != 400 {
			t.Fatal("invalid query", raw, w.Code)
		}
	}
	w = b.request("GET", "/search?q=%3Cscript%3E", nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), `value="<script>"`) || !strings.Contains(w.Body.String(), "No matching music") {
		t.Fatal("query escaping", w.Code, w.Body.String())
	}
	if w := b.request("GET", "/search", nil); w.Code != 200 || strings.Contains(w.Body.String(), "Needle") {
		t.Fatal("blank search", w.Code)
	}
	if w := b.request("HEAD", "/search?q=needle", nil); w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("HEAD contract", w.Code)
	}
	if w := b.request("POST", "/search", url.Values{}); w.Code != 405 {
		t.Fatal("method contract", w.Code)
	}
	guest := httptest.NewRecorder()
	a.ServeHTTP(guest, httptest.NewRequest(http.MethodGet, "/search?q=needle", nil))
	if guest.Code != 303 || strings.Contains(guest.Body.String(), "Needle") {
		t.Fatal("guest search", guest.Code)
	}
}
