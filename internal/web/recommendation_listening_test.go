// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/airencracken/songstead/internal/store"
)

func TestPostingListeningHTTPContract(t *testing.T) {
	a, users := fixture(t)
	author := login(t, a, "alice")
	recipient := login(t, a, "bobby")
	group, err := a.store.SaveGroup(t.Context(), users[0], 0, "Music room", []int64{users[1]})
	if err != nil {
		t.Fatal(err)
	}
	for i, audience := range []string{"members", "person:" + strconv.FormatInt(users[1], 10), "group:" + strconv.FormatInt(group, 10)} {
		w := author.request("POST", "/recommendations/new", url.Values{
			"audience": {audience}, "url": {"https://music.example/http/" + strconv.Itoa(i)},
			// Client-supplied listening state and identity must not override the poster.
			"listening": {"dismissed"}, "sender_id": {strconv.FormatInt(users[1], 10)},
		})
		if w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "/recommendations/") {
			t.Fatal("posting contract", w.Code, w.Body.String())
		}
		path := w.Header().Get("Location")
		for _, viewer := range []struct {
			browser *browser
			status  string
		}{{author, "listened"}, {recipient, "unheard"}} {
			w := viewer.browser.request("GET", path, nil)
			if w.Code != 200 || !strings.Contains(w.Body.String(), `value="`+viewer.status+`" selected`) {
				t.Fatal("detail route listening selection", audience, viewer.status, w.Code, w.Body.String())
			}
		}
	}
	// A rejected post of the same music must leave a later manual choice intact.
	rows, err := a.store.Browse(t.Context(), users[0], false, store.Filter{Recent: true}, 10, 0)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	if err := a.store.React(t.Context(), users[0], rows[0].ID, store.Reaction{Listening: "saved"}); err != nil {
		t.Fatal(err)
	}
	w := author.request("POST", "/recommendations/new", url.Values{"audience": {"members"}, "url": {rows[0].URL}, "note": {strings.Repeat("x", 2001)}})
	if w.Code != 422 {
		t.Fatal("invalid post accepted", w.Code)
	}
	item, err := a.store.Recommendation(t.Context(), users[0], rows[0].ID)
	if err != nil || item.Listening != "saved" {
		t.Fatal("rejected post changed listening", item, err)
	}
}
