// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOwnPostControls(t *testing.T) {
	a, _ := fixture(t)
	alice, bobby := login(t, a, "alice"), login(t, a, "bobby")
	w := alice.request("POST", "/recommendations/new", url.Values{"audience": {"members"}, "url": {"https://example.org/album"}, "title": {"Original title"}})
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	path := w.Header().Get("Location")
	w = alice.request("GET", path+"/edit", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `value="Original title"`) {
		t.Fatal("missing edit form", w.Code, w.Body.String())
	}
	for _, suffix := range []string{"/edit", "/delete"} {
		if w := bobby.request("GET", path+suffix, nil); w.Code != 404 {
			t.Fatal("non-author form", suffix, w.Code)
		}
		if w := bobby.request("POST", path+suffix, url.Values{"url": {"https://example.org/changed"}, "title": {"Stolen"}, "confirm": {"delete"}}); w.Code != 404 {
			t.Fatal("non-author mutation", suffix, w.Code)
		}
	}
	w = alice.request("POST", path+"/edit", url.Values{"url": {"invalid"}, "title": {"Keep this draft"}, "kind": {"album"}})
	if w.Code != 422 || !strings.Contains(w.Body.String(), `value="Keep this draft"`) {
		t.Fatal("invalid draft lost", w.Code, w.Body.String())
	}
	w = alice.request("POST", path+"/edit", url.Values{"url": {"https://example.org/corrected"}, "title": {"Netherworlds"}, "artist": {"Insomnium"}, "kind": {"album"}, "note": {"Corrected post"}, "genre": {"Metal"}, "tags": {"new release"}, "sender_id": {"2"}, "audience": {"private"}})
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, b := range []*browser{alice, bobby} {
		w = b.request("GET", path, nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Netherworlds") || !strings.Contains(w.Body.String(), "Corrected post") {
			t.Fatal(w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), `href="`+path+`/edit"`) != (b == alice) {
			t.Fatal("incorrect author controls")
		}
	}
	w = alice.request("GET", path+"/delete", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Netherworlds") {
		t.Fatal("missing deletion confirmation", w.Code)
	}
	if w := alice.request("POST", path+"/delete", nil); w.Code != 422 {
		t.Fatal("deleted without confirmation", w.Code)
	}
	for _, suffix := range []string{"/edit", "/delete"} {
		r := httptest.NewRequest("POST", path+suffix, strings.NewReader("confirm=delete"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, c := range alice.cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatal("missing CSRF accepted", suffix, w.Code)
		}
	}
	if w := alice.request("POST", path+"/delete", url.Values{"confirm": {"delete"}}); w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, b := range []*browser{alice, bobby} {
		if w := b.request("GET", path, nil); w.Code != 404 {
			t.Fatal("deleted post visible", w.Code)
		}
	}
}
