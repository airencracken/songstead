// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/airencracken/songstead/internal/store"
)

func TestPaginationKeepsEveryFilter(t *testing.T) {
	a, users := fixture(t)
	genre := `R&B "late" <night>`
	for i := range 55 {
		if _, err := a.store.ShareLabeledMusic(t.Context(), users[0], fmt.Sprintf("https://example.org/song/%d", i), "", "track", fmt.Sprintf("Song %d", i), "Artist", store.Labels{Genre: genre}); err != nil {
			t.Fatal(err)
		}
	}
	bobby := login(t, a, "bobby")
	query := url.Values{"genre": {genre}, "view": {"recommendations"}, "discovery": {"all"}, "layout": {"tiles"}, "kind": {"track"}}
	body := bobby.request("GET", "/recent?"+query.Encode(), nil).Body.String()
	match := regexp.MustCompile(`<a href="([^"]+)">Next page</a>`).FindStringSubmatch(body)
	if match == nil {
		t.Fatal("missing next page", body)
	}
	next, err := url.Parse(html.UnescapeString(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	got := next.Query()
	if next.Path != "/recent" || got.Get("offset") != "50" || got.Get("genre") != genre || got.Get("view") != "recommendations" || got.Get("layout") != "tiles" || got.Get("kind") != "track" || got.Get("discovery") != "all" {
		t.Fatal("next page lost filters", next)
	}
	second := bobby.request("GET", next.RequestURI(), nil).Body.String()
	if !strings.Contains(second, "Previous page") || strings.Count(second, `data-music-card=`) != 5 {
		t.Fatal("second page did not follow the filters")
	}
	if strings.Contains(body, "<night>") {
		t.Fatal("genre was not escaped")
	}
}

func TestListHeadingsSayEachThingOnce(t *testing.T) {
	a, _ := fixture(t)
	alice := login(t, a, "alice")
	for _, path := range []string{"/shelf", "/recent", "/history", "/shelf?liked=1"} {
		body := alice.request("GET", path, nil).Body.String()
		if strings.Contains(body, "About this view") {
			t.Errorf("%s repeats its introduction in a disclosure", path)
		}
		if strings.Contains(body, "Your organization") {
			t.Errorf("%s uses an unclear filter label", path)
		}
	}
}

func TestSpoilerSettingsStayTuckedAway(t *testing.T) {
	a, users := fixture(t)
	id, err := a.store.Recommend(t.Context(), users[0], users[1], "https://example.org/song", "")
	if err != nil {
		t.Fatal(err)
	}
	bobby := login(t, a, "bobby")
	path := fmt.Sprintf("/recommendations/%d", id)
	body := bobby.request("GET", path, nil).Body.String()
	if !strings.Contains(body, `<details class="comfy-disclosure spoiler-settings" ><summary>Spoiler settings</summary>`) {
		t.Fatal("spoiler settings are not in a closed disclosure")
	}
	w := bobby.request("POST", path+"/annotations", url.Values{"mode": {"hidden"}})
	if w.Code == 303 {
		w = bobby.request("GET", w.Header().Get("Location"), nil)
	}
	if !strings.Contains(w.Body.String(), `<details class="comfy-disclosure spoiler-settings" open>`) {
		t.Fatal("saved spoiler confirmation is hidden in a closed disclosure")
	}
}
