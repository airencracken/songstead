// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/airencracken/songstead/internal/store"
)

func TestLikedBrowseLinksPreserveMusicContext(t *testing.T) {
	p := page{Recent: true, Liked: true, Layout: "tiles", Perspective: "music", Kind: "track", Genre: "Jazz & soul", Tag: "Live", Discovery: "all", Person: 3, Status: "listened"}
	link, err := url.Parse(browseURL(p, "layout", "chips"))
	if err != nil {
		t.Fatal(err)
	}
	query := link.Query()
	for key, want := range map[string]string{"liked": "1", "layout": "chips", "view": "music", "kind": "track", "genre": "Jazz & soul", "tag": "Live", "discovery": "all", "person": "3", "status": "listened"} {
		if query.Get(key) != want {
			t.Errorf("browse link lost %s: got %q, want %q", key, query.Get(key), want)
		}
	}
	if link.Path != "/recent" || query.Has("offset") {
		t.Fatal("browse switch changed route or kept stale page", link)
	}
	link, err = url.Parse(browseURL(p, "liked", ""))
	if err != nil || link.Query().Get("liked") != "" {
		t.Fatal("cannot leave liked songs", link, err)
	}
}

func TestSongLikeHTTPContractsAndAdversarialInput(t *testing.T) {
	a, users := fixture(t)
	ctx := t.Context()
	viewer := login(t, a, "bobby")
	id, err := a.store.ShareMusic(ctx, users[0], "https://music.example/a", "", "track", "Song", "Artist")
	if err != nil {
		t.Fatal(err)
	}
	path := "/recommendations/" + strconv.FormatInt(id, 10)
	w := viewer.request("GET", path, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `aria-pressed="false">Like song`) || strings.Contains(w.Body.String(), "What did you think?") {
		t.Fatal("song like control", w.Code, w.Body.String())
	}
	for _, back := range []string{"", "https://evil.example/recent", "//evil.example/recent", "/\\evil.example/recent", "/logout", "/recent%0d%0aLocation:evil"} {
		w := viewer.request("POST", path+"/like", url.Values{"liked": {"1"}, "return_to": {back}, "user_id": {strconv.FormatInt(users[0], 10)}})
		if w.Code != 303 || w.Header().Get("Location") != path+"?saved=like#song-like" {
			t.Fatal("unsafe return or unexpected redirect", back, w.Code, w.Header())
		}
	}
	w = viewer.request("GET", path+"?saved=like", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `aria-pressed="true">Unlike song`) || !strings.Contains(w.Body.String(), `data-save-status>Song liked. Private to you.`) {
		t.Fatal("like feedback missing", w.Code, w.Body.String())
	}
	other, err := a.store.Recommendation(ctx, users[0], id)
	if err != nil || other.Liked {
		t.Fatal("posted identity changed other user's like", other, err)
	}
	for _, value := range [][]string{nil, {""}, {"true"}, {"-1"}, {"2"}, {"<script>"}, {"\x00"}, {"1", "0"}} {
		w := viewer.request("POST", path+"/like", url.Values{"liked": value})
		if w.Code != 422 && !(len(value) > 1 && w.Code == 400) {
			t.Fatal("malformed like accepted", value, w.Code)
		}
		item, err := a.store.Recommendation(ctx, users[1], id)
		if err != nil || !item.Liked {
			t.Fatal("invalid request changed like", item, err)
		}
	}
	back := "/recent?layout=tiles&liked=1&view=music&offset=50"
	w = viewer.request("POST", path+"/like", url.Values{"liked": {"0"}, "return_to": {back}})
	if w.Code != 303 || w.Header().Get("Location") != back {
		t.Fatal("unlike lost browse state", w.Code, w.Header())
	}
	w = viewer.request("GET", path+"?saved=like", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Song removed from your likes.") {
		t.Fatal("unlike not confirmed", w.Code, w.Body.String())
	}
	private, err := a.store.Recommend(ctx, users[0], users[2], "https://music.example/a", "SECRET")
	if err != nil {
		t.Fatal(err)
	}
	if w := viewer.request("POST", fmt.Sprintf("/recommendations/%d/like", private), url.Values{"liked": {"1"}}); w.Code != 404 {
		t.Fatal("public duplicate allowed private-context mutation", w.Code)
	}
	if w := viewer.request("GET", path+"/like", nil); w.Code != 405 {
		t.Fatal("GET mutated a song like", w.Code)
	}
	if w := viewer.request("POST", "/recommendations/nope/like", url.Values{"liked": {"1"}}); w.Code != 404 {
		t.Fatal("invalid route id accepted", w.Code)
	}
	request := httptest.NewRequest("POST", path+"/like", strings.NewReader("liked=1"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range viewer.cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal("like bypassed CSRF", response.Code)
	}
	guest := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	if w := guest.request("POST", path+"/like", url.Values{"liked": {"1"}}); w.Code != 303 && w.Code != 403 {
		t.Fatal("anonymous like endpoint", w.Code)
	}
}

func TestDefaultSongDeduplicationKeepsDatedProvenance(t *testing.T) {
	a, users := fixture(t)
	ctx := t.Context()
	viewer := login(t, a, "bobby")
	first, err := a.store.ShareMusic(ctx, users[0], "https://youtu.be/dQw4w9WgXcQ", "Alice's recommendation", "track", "A shared song", "Artist")
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.store.ShareMusic(ctx, users[2], "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=30", "Carol's recommendation", "track", "A shared song", "Artist")
	if err != nil {
		t.Fatal(err)
	}
	private, err := a.store.RecommendMusic(ctx, users[0], users[2], 0, "https://youtu.be/dQw4w9WgXcQ", "SECRET recommendation", "track", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, layout := range []string{"chips", "tiles"} {
		w := viewer.request("GET", "/recent?layout="+layout, nil)
		body := w.Body.String()
		if w.Code != 200 || strings.Count(body, "data-song-id=") != 1 || strings.Count(body, "data-music-title") != 1 || strings.Count(body, ">Like song</button>") != 1 || strings.Count(body, "<time datetime=") != 2 {
			t.Fatal("duplicate song card or lost dates", layout, w.Code, body)
		}
		for _, text := range []string{"From alice", "From carol", "Alice&#39;s recommendation", "Carol&#39;s recommendation", fmt.Sprintf("href=\"/recommendations/%d\"", first), fmt.Sprintf("href=\"/recommendations/%d\"", second)} {
			if !strings.Contains(body, text) {
				t.Fatal("missing recommendation provenance", text, body)
			}
		}
		if strings.Contains(body, "SECRET") || strings.Contains(body, fmt.Sprintf("/recommendations/%d\"", private)) {
			t.Fatal("grouping leaked a private recommendation", body)
		}
	}
	if err := a.store.SetLiked(ctx, users[1], first, true); err != nil {
		t.Fatal(err)
	}
	w := viewer.request("GET", "/shelf?liked=1", nil)
	if w.Code != 200 || strings.Count(w.Body.String(), "data-song-id=") != 1 || !strings.Contains(w.Body.String(), "Your liked songs") || !strings.Contains(w.Body.String(), "From carol") {
		t.Fatal("liked public song missing from personal shelf", w.Code, w.Body.String())
	}
	for _, query := range []string{"liked=bad", "liked=-1", "liked=1&liked=0"} {
		if w := viewer.request("GET", "/shelf?"+query, nil); w.Code != 400 {
			t.Fatal("invalid likes filter accepted", query, w.Code)
		}
	}
	// Different provider URLs stay distinct even with identical metadata.
	if _, err := a.store.ShareMusic(ctx, users[0], "https://open.spotify.com/track/4uLU6hMCjMI75M1A2tKUQC", "Spotify version", "track", "A shared song", "Artist"); err != nil {
		t.Fatal(err)
	}
	w = viewer.request("GET", "/recent", nil)
	if w.Code != 200 || strings.Count(w.Body.String(), "data-song-id=") != 2 {
		t.Fatal("different providers were merged", w.Code, w.Body.String())
	}
}

func TestGroupedSongPaginationCountsSongsAndPreservesFilters(t *testing.T) {
	a, users := fixture(t)
	ctx := t.Context()
	viewer := login(t, a, "bobby")
	for i := 0; i < 55; i++ {
		if _, err := a.store.ShareMusic(ctx, users[i%2], "https://music.example/duplicate", "Duplicate "+strconv.Itoa(i), "", "Repeated song", ""); err != nil {
			t.Fatal(err)
		}
	}
	w := viewer.request("GET", "/recent", nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "Next page") || strings.Count(w.Body.String(), "<time datetime=") != 55 || strings.Count(w.Body.String(), "data-song-id=") != 1 {
		t.Fatal("duplicate contexts consumed song pages", w.Code, w.Body.String())
	}
	for i := 0; i < 50; i++ {
		id, err := a.store.ShareMusic(ctx, users[0], fmt.Sprintf("https://music.example/unique/%d", i), "Unique", "", "Song", "")
		if err != nil || a.store.SetLiked(ctx, users[1], id, true) != nil {
			t.Fatal("could not create liked song", err)
		}
	}
	rows, err := a.store.Browse(ctx, users[1], false, store.Filter{Recent: true, Person: users[1]}, 1, 0)
	if err != nil || len(rows) != 1 || a.store.SetLiked(ctx, users[1], rows[0].ID, true) != nil {
		t.Fatal(rows, err)
	}
	w = viewer.request("GET", "/recent?liked=1&layout=tiles", nil)
	body := w.Body.String()
	if w.Code != 200 || strings.Count(body, "data-song-id=") != 50 || !strings.Contains(body, "Next page") || !strings.Contains(body, "liked=1") {
		t.Fatal("song page or like filter lost", w.Code, body)
	}
	w = viewer.request("GET", "/recent?liked=1&layout=tiles&offset=50", nil)
	if w.Code != 200 || strings.Count(w.Body.String(), "data-song-id=") != 1 || strings.Count(w.Body.String(), "<time datetime=") != 55 || strings.Contains(w.Body.String(), "Next page") || !strings.Contains(w.Body.String(), "Previous page") {
		t.Fatal("older page lost duplicate recommendations", w.Code, w.Body.String())
	}
}
