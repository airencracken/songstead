// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"testing/quick"

	"github.com/airencracken/songstead/internal/media"
)

func TestPreviewContractsPrivacyAndRateLimit(t *testing.T) {
	a, u := fixture(t)
	alice := login(t, a, "alice")
	carol := login(t, a, "carol")
	calls := 0
	a.previewFetch = func(_ context.Context, raw string) (media.Metadata, error) {
		calls++
		if strings.Contains(raw, "aaaaaaaaaaa") {
			return media.Metadata{}, errors.New("offline")
		}
		return media.Metadata{Title: "A title <script>", Artist: "An artist", Artwork: []byte("png")}, nil
	}
	for _, raw := range []string{"https://localhost/private", "https://youtube.com.attacker.example/watch?v=dQw4w9WgXcQ", "https://music.example/song"} {
		w := alice.request("POST", "/recommendations/preview", url.Values{"url": {raw}})
		if w.Code != 200 || !strings.Contains(w.Body.String(), "unsupported") || calls != 0 {
			t.Fatal("unexpected fetch", w.Code, calls)
		}
	}
	w := alice.request("POST", "/recommendations/preview", url.Values{"url": {"javascript:bad"}})
	if w.Code != 422 || calls != 0 {
		t.Fatal("invalid URL fetched")
	}
	w = alice.request("POST", "/recommendations/preview", url.Values{"url": {"https://youtu.be/dQw4w9WgXcQ"}})
	var data struct {
		Status, Title, Artist string
		Artwork               []byte
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil || w.Code != 200 || data.Status != "ready" || data.Title != "A title <script>" || string(data.Artwork) != "png" {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	w = alice.request("POST", "/recommendations/preview", url.Values{"url": {"https://youtu.be/aaaaaaaaaaa"}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "unavailable") {
		t.Fatal("fetch failure contract", w.Code)
	}
	for n := 0; n < 4; n++ {
		alice.request("POST", "/recommendations/preview", url.Values{"url": {"https://youtu.be/dQw4w9WgXcQ"}})
	}
	w = alice.request("POST", "/recommendations/preview", url.Values{"url": {"https://youtu.be/dQw4w9WgXcQ"}})
	if w.Code != 429 || calls != 6 {
		t.Fatal("preview rate limit", w.Code, calls)
	}
	id, err := a.store.Recommend(t.Context(), u[0], u[1], "https://youtu.be/dQw4w9WgXcQ", "Private")
	if err != nil {
		t.Fatal(err)
	}
	path := "/recommendations/" + strconv.FormatInt(id, 10)
	w = carol.request("GET", path+"/preview", nil)
	if w.Code != 404 || strings.Contains(w.Body.String(), "Private") {
		t.Fatal("private preview leaked", w.Code)
	}
	w = carol.request("POST", path+"/preview", nil)
	if w.Code != 404 {
		t.Fatal("unauthorized refresh", w.Code)
	}
	anon := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	for _, route := range []string{path + "/preview", "/users/1/picture"} {
		if w = anon.request("GET", route, nil); w.Code != 303 {
			t.Fatal("anonymous preview", route, w.Code)
		}
	}
	w = anon.request("GET", "/login", nil)
	if !strings.Contains(w.Body.String(), `property="og:image" content="http://example.com/static/jukebox.png"`) || !strings.Contains(w.Body.String(), `property="og:title" content="Songstead"`) || strings.Contains(w.Body.String(), "Private") {
		t.Fatal("unsafe social card")
	}
	w = alice.request("GET", path, nil)
	if !strings.Contains(w.Body.String(), `referrerpolicy="strict-origin-when-cross-origin"`) || strings.Contains(w.Body.String(), `referrerpolicy="no-referrer"`) || !strings.Contains(w.Body.String(), `target="_blank" rel="noopener noreferrer"`) {
		t.Fatal("embed referrer or new-tab contract")
	}
}

func TestProfileUploadRoutesAndViewerPreference(t *testing.T) {
	a, u := fixture(t)
	alice := login(t, a, "alice")
	bobby := login(t, a, "bobby")
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	if err := writer.WriteField("csrf", alice.csrf); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("user", "2"); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("picture", "picture.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write(imageData.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/account/picture", &form)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	for _, cookie := range alice.cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	p, err := a.store.Profile(t.Context(), u[0])
	if err != nil || len(p.Still) == 0 {
		t.Fatal("upload absent", err)
	}
	p, err = a.store.Profile(t.Context(), u[1])
	if err != nil || len(p.Still) > 0 {
		t.Fatal("upload targeted another user", err)
	}
	w = bobby.request("GET", "/users/1/picture", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("picture headers", w.Code, w.Header())
	}
	w = bobby.request("HEAD", "/users/1/picture?still=1", nil)
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("picture HEAD", w.Code)
	}
	w = bobby.request("POST", "/account/animation", nil)
	if w.Code != 303 {
		t.Fatal(w.Code)
	}
	p, _ = a.store.Profile(t.Context(), u[1])
	if p.Animate {
		t.Fatal("preference absent")
	}
	w = bobby.request("POST", "/account/animation", url.Values{"animate": {"bogus"}})
	if w.Code != 422 {
		t.Fatal("invalid preference accepted")
	}
	w = alice.request("POST", "/account/picture", url.Values{"remove_picture": {"1"}})
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	p, _ = a.store.Profile(t.Context(), u[0])
	if len(p.Still) > 0 {
		t.Fatal("remove failed")
	}
}

func TestBrowseLinksPreserveFiltersAndEscapeNames(t *testing.T) {
	if err := quick.Check(func(value string) bool {
		p := page{Recent: true, Layout: "tiles", Genre: "Jazz", Tag: "Live", Discovery: "all", Perspective: "person", Person: 42, GroupID: 12, Kind: "album", Status: "saved", Offset: 50}
		raw := browseURL(p, "genre", value)
		u, err := url.Parse(raw)
		if err != nil {
			return false
		}
		q := u.Query()
		return u.Path == "/recent" && q.Get("genre") == value && q.Get("tag") == "Live" && q.Get("layout") == "tiles" && q.Get("view") == "person" && q.Get("person") == "42" && q.Get("group") == "12" && q.Get("kind") == "album" && q.Get("discovery") == "all" && q.Get("status") == "saved" && q.Get("offset") == ""
	}, nil); err != nil {
		t.Fatal(err)
	}
}
