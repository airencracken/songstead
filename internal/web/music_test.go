// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/airencracken/songstead/internal/media"
	"github.com/airencracken/songstead/internal/store"
)

func TestAccountSpoilerPreferencesAndFeedbackNotice(t *testing.T) {
	a, u := fixture(t)
	b := login(t, a, "bobby")
	id, err := a.store.Recommend(t.Context(), u[0], u[1], "https://music.example/song", "Listen when you like")
	if err != nil {
		t.Fatal(err)
	}
	path := "/recommendations/" + strconv.FormatInt(id, 10)
	w := b.request("GET", "/account", nil)
	for _, want := range []string{`>Account</a>`, `name="mode"`, `Spoiler preferences`, `action="/account/annotations"`} {
		if w.Code != 200 || !strings.Contains(w.Body.String(), want) {
			t.Fatal("undiscoverable preferences", want, w.Code)
		}
	}
	w = b.request("POST", "/account/annotations", url.Values{"mode": {"spoiler-free"}, "user": {"1"}})
	if w.Code != 303 || w.Header().Get("Location") != "/account?saved=annotations#spoiler-preferences" {
		t.Fatal(w.Code, w.Header())
	}
	w = b.request("GET", w.Header().Get("Location"), nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `value="spoiler-free" selected`) || !strings.Contains(w.Body.String(), `role="status"`) || !strings.Contains(w.Body.String(), "Spoiler preference saved") {
		t.Fatal(w.Code, w.Body.String())
	}
	mode, _ := a.store.AnnotationMode(t.Context(), u[0])
	if mode != "immediate" {
		t.Fatal("preference targeted another user")
	}
	w = b.request("POST", "/account/annotations", url.Values{"mode": {"<script>"}})
	if w.Code != 422 || strings.Contains(w.Body.String(), "preference saved") || !strings.Contains(w.Body.String(), `value="spoiler-free" selected`) {
		t.Fatal("invalid preference falsely confirmed", w.Code, w.Body.String())
	}
	w = b.request("POST", path+"/reaction", url.Values{"listening": {"listened"}, "rating": {"1"}, "personal_note": {"Private feedback"}})
	if w.Code != 303 || w.Header().Get("Location") != path+"?saved=feedback#listening-notes" {
		t.Fatal("save lacks confirmed redirect", w.Code, w.Header())
	}
	w = b.request("GET", w.Header().Get("Location"), nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `id="listening-notes"`) || !strings.Contains(w.Body.String(), `data-save-status>Listening feedback saved.`) || !strings.Contains(w.Body.String(), "Private feedback") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = b.request("POST", path+"/reaction", url.Values{"listening": {"revisit"}, "rating": {"bad"}, "personal_note": {"Keep this draft <script>"}})
	if w.Code != 422 || !strings.Contains(w.Body.String(), "Keep this draft &lt;script&gt;") || strings.Contains(w.Body.String(), "Listening feedback saved") {
		t.Fatal("failed feedback lost draft or confirmed", w.Code, w.Body.String())
	}
	item, _ := a.store.Recommendation(t.Context(), u[1], id)
	if item.PersonalNote != "Private feedback" || item.Listening != "listened" {
		t.Fatal("validation partially persisted", item)
	}
	w = b.request("GET", path+"?saved="+url.QueryEscape("<script>arbitrary</script>"), nil)
	if strings.Contains(w.Body.String(), "arbitrary") || strings.Contains(w.Body.String(), `role="status"`) {
		t.Fatal("arbitrary URL generated confirmation")
	}
	w = b.request("POST", path+"/annotations", url.Values{"mode": {"hidden"}})
	if w.Code != 303 || !strings.Contains(w.Header().Get("Location"), "saved=annotations") {
		t.Fatal(w.Code, w.Header())
	}
	w = b.request("POST", "/account/password", url.Values{"current_password": {"wrong"}, "password": {"a-valid-new-password"}, "confirmation": {"a-valid-new-password"}})
	if w.Code != 422 || !strings.Contains(w.Body.String(), `value="hidden" selected`) {
		t.Fatal("password errors lost spoiler preference", w.Code)
	}
}

func TestLabelsDiscoveryAndLayoutHTTPContracts(t *testing.T) {
	a, u := fixture(t)
	alice := login(t, a, "alice")
	bobby := login(t, a, "bobby")
	w := alice.request("POST", "/recommendations/new", url.Values{"audience": {"members"}, "url": {"https://music.example/a"}, "title": {"Jazz record"}, "genre": {"Jazz"}, "tags": {"Live, acoustic"}})
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	path := w.Header().Get("Location")
	w = bobby.request("GET", path, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), ">Jazz</a>") || !strings.Contains(w.Body.String(), ">acoustic</a>") || strings.Contains(w.Body.String(), "Save genre and tags") {
		t.Fatal("labels absent or editable by recipient", w.Code, w.Body.String())
	}
	w = bobby.request("POST", path+"/labels", url.Values{"genre": {"Changed"}, "tags": {"Changed"}})
	if w.Code != 404 {
		t.Fatal("nonsender edited labels", w.Code)
	}
	w = alice.request("POST", path+"/labels", url.Values{"genre": {"Ambient"}, "tags": {"Instrumental"}})
	if w.Code != 303 || !strings.Contains(w.Header().Get("Location"), "saved=labels") {
		t.Fatal(w.Code, w.Header())
	}
	w = alice.request("GET", w.Header().Get("Location"), nil)
	if !strings.Contains(w.Body.String(), "Genre and tags saved.") {
		t.Fatal(w.Body.String())
	}
	w = bobby.request("POST", "/account/discovery", url.Values{"excluded_genres": {"ambient"}, "preferred_tags": {"instrumental"}, "user_id": {strconv.FormatInt(u[0], 10)}})
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = bobby.request("GET", w.Header().Get("Location"), nil)
	if !strings.Contains(w.Body.String(), "Discovery preferences saved.") || !strings.Contains(w.Body.String(), `name="excluded_genres" value="ambient"`) {
		t.Fatal(w.Body.String())
	}
	for _, layout := range []string{"chips", "tiles"} {
		w = bobby.request("GET", "/recent?layout="+layout, nil)
		if w.Code != 200 || strings.Contains(w.Body.String(), "Jazz record") {
			t.Fatal("exclusion did not apply", layout, w.Code)
		}
		w = bobby.request("GET", "/recent?layout="+layout+"&discovery=all", nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Jazz record") || !strings.Contains(w.Body.String(), `class="recommendations `+layout+`"`) {
			t.Fatal("all music or layout missing", w.Code, w.Body.String())
		}
		if (layout == "tiles") != strings.Contains(w.Body.String(), `class="tile-artwork"`) {
			t.Fatal("tile fallback missing or leaked into chips")
		}
	}
	w = alice.request("GET", "/recent", nil)
	if !strings.Contains(w.Body.String(), "Jazz record") || !strings.Contains(w.Body.String(), `class="recommendations chips"`) {
		t.Fatal("private preferences leaked or default changed")
	}
	w = bobby.request("POST", "/account/discovery", url.Values{"excluded_genres": {"Metal"}, "preferred_tags": {"bad\nname"}})
	if w.Code != 422 || !strings.Contains(w.Body.String(), `name="excluded_genres" value="Metal"`) || strings.Contains(w.Body.String(), "Discovery preferences saved") {
		t.Fatal("invalid discovery draft lost/confirmed", w.Code)
	}
	w = bobby.request("GET", "/account", nil)
	if !strings.Contains(w.Body.String(), `name="excluded_genres" value="ambient"`) {
		t.Fatal("invalid preferences partially saved")
	}
	for _, query := range []string{"layout=evil", "discovery=evil", "genre=" + url.QueryEscape(strings.Repeat("x", 81)), "tag=" + url.QueryEscape("bad\nname")} {
		w = bobby.request("GET", "/recent?"+query, nil)
		if w.Code != 400 && w.Code != 422 {
			t.Fatal("malformed filter accepted", query, w.Code)
		}
	}
	w = bobby.request("GET", "/account/export", nil)
	var archive store.Archive
	if err := json.Unmarshal(w.Body.Bytes(), &archive); err != nil || w.Code != 200 || len(archive.DiscoveryPreferences.ExcludedGenres) != 1 || archive.DiscoveryPreferences.ExcludedGenres[0] != "ambient" {
		t.Fatal("export missed preferences", w.Code, err)
	}
}

func TestThumbnailRoutesAccessAndHeaders(t *testing.T) {
	a, u := fixture(t)
	id, err := a.store.Recommend(t.Context(), u[0], u[1], "https://youtu.be/dQw4w9WgXcQ", "Private artwork")
	if err != nil {
		t.Fatal(err)
	}
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err := a.store.Metadata(t.Context(), func(_ context.Context, _ string) (media.Metadata, error) {
		return media.Metadata{Title: "Song", Artwork: imageBytes.Bytes()}, nil
	}); err != nil {
		t.Fatal(err)
	}
	path := "/recommendations/" + strconv.FormatInt(id, 10)
	b := login(t, a, "bobby")
	for _, method := range []string{"GET", "HEAD"} {
		w := b.request(method, path+"/thumbnail", nil)
		if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Length") != strconv.Itoa(imageBytes.Len()) {
			t.Fatal("image HTTP contract", method, w.Code, w.Header())
		}
		if method == "GET" && !bytes.Equal(w.Body.Bytes(), imageBytes.Bytes()) || method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("image body contract", method, w.Body.Len())
		}
	}
	w := b.request("GET", path, nil)
	if !strings.Contains(w.Body.String(), `class="detail-artwork" src="`+path+`/thumbnail"`) || !strings.Contains(w.Header().Get("Content-Security-Policy"), "img-src 'self' blob:;") {
		t.Fatal("detail missed local artwork", w.Code, w.Body.String())
	}
	carol := login(t, a, "carol")
	if w := carol.request("GET", path+"/thumbnail", nil); w.Code != 404 || bytes.Contains(w.Body.Bytes(), imageBytes.Bytes()) {
		t.Fatal("thumbnail exposed to stranger", w.Code)
	}
	anon := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	if w := anon.request("GET", path+"/thumbnail", nil); w.Code != 303 {
		t.Fatal("anonymous image access", w.Code)
	}
	for _, route := range []string{"POST /account/annotations", "POST /account/discovery", "POST " + path + "/labels"} {
		method, path, _ := strings.Cut(route, " ")
		req := httptest.NewRequest(method, path, strings.NewReader("mode=hidden"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, cookie := range b.cookies {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatal("new route bypassed CSRF", route, w.Code)
		}
	}
}
