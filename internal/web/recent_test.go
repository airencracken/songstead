// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/airencracken/songstead/internal/store"
)

func TestRecentAudienceBrowserFlow(t *testing.T) {
	a, u := fixture(t)
	alice := login(t, a, "alice")
	bobby := login(t, a, "bobby")
	carol := login(t, a, "carol")
	group, err := a.store.SaveGroup(t.Context(), u[0], 0, "Private circle", []int64{u[1]})
	if err != nil {
		t.Fatal(err)
	}
	w := alice.request("GET", "/recommendations/new", nil)
	for _, text := range []string{`name="audience"`, `value="members" selected`, `value="person:` + strconv.FormatInt(u[1], 10) + `"`, `value="group:` + strconv.FormatInt(group, 10) + `"`, "never appear in Recent"} {
		if w.Code != 200 || !strings.Contains(w.Body.String(), text) {
			t.Fatal("native audience form missing choice/help", text, w.Code, w.Body.String())
		}
	}
	w = alice.request("POST", "/recommendations/new", url.Values{"audience": {"members"}, "url": {"https://music.example/shared"}, "kind": {"album"}, "title": {"Shared album"}, "note": {"Something for everyone"}})
	if w.Code != 303 {
		t.Fatal("shared posting failed", w.Code, w.Body.String())
	}
	sharedPath := w.Header().Get("Location")
	w = alice.request("POST", "/recommendations/new", url.Values{"audience": {"person:" + strconv.FormatInt(u[1], 10)}, "url": {"https://music.example/shared"}, "note": {"Friend secret"}})
	if w.Code != 303 {
		t.Fatal("private posting failed", w.Code, w.Body.String())
	}
	privatePath := w.Header().Get("Location")
	w = alice.request("POST", "/recommendations/new", url.Values{"audience": {"group:" + strconv.FormatInt(group, 10)}, "url": {"https://music.example/shared"}, "note": {"Group secret"}})
	if w.Code != 303 {
		t.Fatal("group posting failed", w.Code, w.Body.String())
	}
	groupPath := w.Header().Get("Location")
	for _, b := range []*browser{alice, bobby, carol} {
		for _, view := range []string{"", "?view=music", "?view=person", "?view=group", "?view=recommendations"} {
			w := b.request("GET", "/recent"+view, nil)
			if w.Code != 200 || !strings.Contains(w.Body.String(), "Something for everyone") || strings.Contains(w.Body.String(), "Friend secret") || strings.Contains(w.Body.String(), "Group secret") || strings.Contains(w.Body.String(), `href="`+privatePath+`"`) || strings.Contains(w.Body.String(), `href="`+groupPath+`"`) {
				t.Fatal("Recent audience boundary", view, w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), `href="/recent" aria-current="page"`) || !strings.Contains(w.Body.String(), "From alice") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("Recent navigation, provenance or caching contract", w.Body.String())
			}
		}
	}
	if w := carol.request("GET", sharedPath, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "shared this with everyone here") || !strings.Contains(w.Body.String(), "Comments are visible to everyone signed in here") {
		t.Fatal("shared discussion audience unclear", w.Code, w.Body.String())
	}
	if w := carol.request("POST", sharedPath+"/comments", url.Values{"body": {"A shared comment"}}); w.Code != 303 {
		t.Fatal("shared comment unavailable", w.Code, w.Body.String())
	}
	if w := carol.request("POST", sharedPath+"/reaction", url.Values{"listening": {"saved"}, "rating": {"1"}, "personal_note": {"Carol private organization"}}); w.Code != 303 {
		t.Fatal("shared private organization unavailable", w.Code, w.Body.String())
	}
	for _, path := range []string{"/recent", sharedPath, "/account/export"} {
		w := bobby.request("GET", path, nil)
		if w.Code != 200 || strings.Contains(w.Body.String(), "Carol private organization") {
			t.Fatal("shared post leaked personal notes", path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{privatePath, groupPath} {
		if w := carol.request("GET", path, nil); w.Code != 404 {
			t.Fatal("private detail accessible alongside public duplicate", w.Code)
		}
		if w := carol.request("POST", path+"/comments", url.Values{"body": {"intruder"}}); w.Code != 404 {
			t.Fatal("private comment write allowed", w.Code)
		}
	}
	w = bobby.request("GET", "/inbox", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Friend secret") || !strings.Contains(w.Body.String(), "Group secret") || strings.Contains(w.Body.String(), "Something for everyone") {
		t.Fatal("inbox lost private gifts or added shared assignments", w.Code, w.Body.String())
	}
	anon := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	for _, path := range []string{"/recent", sharedPath} {
		if w := anon.request("GET", path, nil); w.Code != 303 || w.Header().Get("Location") != "/login" || strings.Contains(w.Body.String(), "Something for everyone") {
			t.Fatal("shared music exposed anonymously", path, w.Code)
		}
	}
	if w := carol.request("HEAD", "/recent", nil); w.Code != 200 {
		t.Fatal("Recent HEAD route", w.Code)
	}
	if w := carol.request("POST", "/recent", nil); w.Code != 405 {
		t.Fatal("Recent accepted posting", w.Code)
	}
}

func TestRecentLegacyAndAdversarialAudience(t *testing.T) {
	a, u := fixture(t)
	alice := login(t, a, "alice")
	group, err := a.store.SaveGroup(t.Context(), u[0], 0, "Old group", []int64{u[1]})
	if err != nil {
		t.Fatal(err)
	}
	for _, form := range []url.Values{
		{"recipient": {strconv.FormatInt(u[1], 10)}, "url": {"https://music.example/old"}, "note": {"Legacy friend gift"}},
		{"group": {strconv.FormatInt(group, 10)}, "url": {"https://music.example/old"}, "note": {"Legacy group gift"}},
	} {
		w := alice.request("POST", "/recommendations/new", form)
		if w.Code != 303 {
			t.Fatal("legacy private form failed", w.Code, w.Body.String())
		}
		if w := alice.request("GET", "/recent", nil); w.Code != 200 || strings.Contains(w.Body.String(), form.Get("note")) {
			t.Fatal("legacy form broadened audience", w.Code, w.Body.String())
		}
	}
	for _, audience := range []string{"", "public", "members:1", "person:0", "person:-1", "person:nope", "person:9223372036854775808", "person:999999", "person:" + strconv.FormatInt(u[0], 10), "group:999999", "person:1:2", "group:-1", "<script>alert(1)</script>"} {
		w := alice.request("POST", "/recommendations/new", url.Values{"audience": {audience}, "url": {"https://music.example/rejected"}, "note": {"Rejected draft"}})
		if w.Code != 422 && w.Code != 404 {
			t.Fatal("invalid audience accepted", audience, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), `value="members" selected`) || strings.Contains(w.Body.String(), "<script>alert(1)</script>") {
			t.Fatal("invalid draft became shared or executed HTML", audience, w.Body.String())
		}
	}
	for _, field := range []string{"recipient", "group"} {
		w := alice.request("POST", "/recommendations/new", url.Values{"audience": {"members"}, field: {"1"}, "url": {"https://music.example/mixed"}})
		if w.Code != 422 || strings.Contains(w.Body.String(), `value="members" selected`) {
			t.Fatal("mixed audience controls accepted", field, w.Code)
		}
	}
	if w := alice.request("POST", "/recommendations/new", url.Values{"audience": {"members", "person:2"}, "url": {"https://music.example/duplicate"}}); w.Code != 400 {
		t.Fatal("duplicate audience accepted", w.Code)
	}
	for _, audience := range []string{"person:" + strconv.FormatInt(u[1], 10), "group:" + strconv.FormatInt(group, 10)} {
		w := alice.request("POST", "/recommendations/new", url.Values{"audience": {audience}, "url": {"javascript:alert(1)"}, "note": {"Keep my private draft"}})
		if w.Code != 422 || !strings.Contains(w.Body.String(), `value="`+audience+`" selected`) || !strings.Contains(w.Body.String(), "Keep my private draft") || strings.Contains(w.Body.String(), `value="members" selected`) {
			t.Fatal("failed private draft lost its audience", w.Code, w.Body.String())
		}
	}
	w := alice.request("GET", "/account/export", nil)
	var archive store.Archive
	if err := json.Unmarshal(w.Body.Bytes(), &archive); err != nil || len(archive.Recommendations) != 2 {
		t.Fatal("rejected forms wrote recommendations", err, w.Body.String())
	}
	request := httptest.NewRequest("POST", "/recommendations/new", strings.NewReader(url.Values{"audience": {"members"}, "url": {"https://music.example/forged"}}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range alice.cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal("shared post bypassed CSRF", response.Code)
	}
}

func TestRecentPaginationAndFilters(t *testing.T) {
	a, u := fixture(t)
	carol := login(t, a, "carol")
	for i := 0; i < 51; i++ {
		if _, err := a.store.ShareMusic(t.Context(), u[0], "https://music.example/"+strconv.Itoa(i), "Post "+strconv.Itoa(i), "album", "Album", ""); err != nil {
			t.Fatal(err)
		}
	}
	w := carol.request("GET", "/recent?kind=album&person="+strconv.FormatInt(u[0], 10), nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Next page") || strings.Contains(w.Body.String(), "Post 0</blockquote>") || strings.Index(w.Body.String(), "Post 50</blockquote>") > strings.Index(w.Body.String(), "Post 49</blockquote>") {
		t.Fatal("Recent ordering/pagination contract", w.Code, w.Body.String())
	}
	w = carol.request("GET", "/recent?offset=50&kind=album&person="+strconv.FormatInt(u[0], 10), nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Post 0</blockquote>") || strings.Contains(w.Body.String(), "Next page") || !strings.Contains(w.Body.String(), "Previous page") {
		t.Fatal("Recent second page", w.Code, w.Body.String())
	}
	for _, query := range []string{"person=-1", "group=-1", "kind=evil", "view=evil", "offset=-1"} {
		if w := carol.request("GET", "/recent?"+query, nil); w.Code != 400 {
			t.Fatal("Recent accepted malformed filter", query, w.Code)
		}
	}
	if w := carol.request("GET", "/recent?kind=track", nil); w.Code != 200 || strings.Contains(w.Body.String(), "Post 50") {
		t.Fatal("Recent kind filter", w.Code)
	}
}

func TestShelfNameAndLegacyRoute(t *testing.T) {
	a, u := fixture(t)
	bobby := login(t, a, "bobby")
	if _, err := a.store.Recommend(t.Context(), u[0], u[1], "https://music.example/gift", "A gift for the shelf"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/shelf", "/inbox", "/shelf?view=person", "/inbox?view=person"} {
		w := bobby.request("GET", path, nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Your shelf") || !strings.Contains(w.Body.String(), "A gift for the shelf") || strings.Contains(strings.ToLower(w.Body.String()), "inbox") || !strings.Contains(w.Body.String(), `href="/shelf" aria-current="page"`) {
			t.Fatal("shelf name/navigation or legacy route", path, w.Code, w.Body.String())
		}
	}
	if w := bobby.request("GET", "/shelf?status=evil", nil); w.Code != 422 {
		t.Fatal("shelf bypassed filter validation", w.Code)
	}
	anon := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	if w := anon.request("GET", "/shelf", nil); w.Code != 303 {
		t.Fatal("shelf anonymous access", w.Code)
	}
}
