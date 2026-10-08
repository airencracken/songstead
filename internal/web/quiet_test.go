// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"fmt"
	"github.com/airencracken/songstead/internal/store"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestSpoilersNeverEnterConcealedHTML(t *testing.T) {
	a, u := fixture(t)
	id, err := a.store.Recommend(t.Context(), u[0], u[1], "https://youtu.be/dQw4w9WgXcQ", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.store.AddComment(t.Context(), u[0], id, "SECRET-ENDING at 0:10 and 0:40"); err != nil {
		t.Fatal(err)
	}
	tracks, _ := a.store.Recordings(t.Context(), u[1], id)
	b := login(t, a, "bobby")
	path := "/recommendations/" + strconv.FormatInt(id, 10)
	a.store.SetAnnotationMode(t.Context(), u[1], "spoiler-free")
	a.store.SetPosition(t.Context(), u[1], id, tracks[0].ID, 10)
	w := b.request("GET", path, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "SECRET-ENDING") {
		t.Fatal("spoiler in HTML", w.Code, w.Body.String())
	}
	a.store.SetPosition(t.Context(), u[1], id, tracks[0].ID, 40)
	w = b.request("GET", path, nil)
	if !strings.Contains(w.Body.String(), "SECRET-ENDING") || !strings.Contains(w.Body.String(), "href=\"#comment-") {
		t.Fatal("reached annotation absent")
	}
	a.store.SetAnnotationMode(t.Context(), u[1], "hidden")
	w = b.request("GET", path, nil)
	if strings.Contains(w.Body.String(), "SECRET-ENDING") {
		t.Fatal("hidden prose present")
	}
	w = b.request("GET", path+"?reveal=1", nil)
	if !strings.Contains(w.Body.String(), "SECRET-ENDING") {
		t.Fatal("explicit reveal absent")
	}
	sender, _ := a.store.AnnotationMode(t.Context(), u[0])
	if sender != "immediate" {
		t.Fatal("preference leaked")
	}
}
func TestMusicViewRetainsGiftContextsWithoutDuplicateListening(t *testing.T) {
	a, u := fixture(t)
	a.store.Recommend(t.Context(), u[0], u[1], "https://youtu.be/dQw4w9WgXcQ", "alice-note")
	a.store.Recommend(t.Context(), u[2], u[1], "https://youtube.com/watch?v=dQw4w9WgXcQ", "carol-note")
	b := login(t, a, "bobby")
	w := b.request("GET", "/inbox", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "alice-note") || !strings.Contains(w.Body.String(), "carol-note") || strings.Count(w.Body.String(), "Open on youtube") != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, view := range []string{"person", "group", "recommendations"} {
		w = b.request("GET", "/inbox?view="+view, nil)
		if w.Code != 200 {
			t.Fatal(view, w.Code)
		}
	}
	for _, query := range []string{"view=bad", "person=-1", "person=nope", "group=-2", "kind=bad"} {
		if w = b.request("GET", "/inbox?"+query, nil); w.Code != 400 {
			t.Fatal(query, w.Code)
		}
	}
}
func TestExplicitHandoffExcludesPrivateContextAndDoesNotPost(t *testing.T) {
	a, u := fixture(t)
	a.config.WitmootURL = "https://board.example"
	a.config.BaseURL = "https://music.example"
	id, _ := a.store.Recommend(t.Context(), u[0], u[1], "https://youtu.be/dQw4w9WgXcQ", "PRIVATE-FRIEND-NOTE")
	a.store.React(t.Context(), u[1], id, store.Reaction{Listening: "saved", Note: "PRIVATE-MY-NOTE"})
	b := login(t, a, "bobby")
	path := "/recommendations/" + strconv.FormatInt(id, 10)
	w := b.request("POST", path+"/share", url.Values{})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "https://board.example/share?") {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "PRIVATE-") {
		t.Fatal("private context handed off")
	}
	c := login(t, a, "carol")
	for _, route := range []string{"share", "discussion", "position", "recordings", "annotations"} {
		w = c.request("POST", path+"/"+route, url.Values{"url": {"https://board.example/topics/2"}, "mode": {"hidden"}, "recording": {"1"}, "position": {"0:10"}, "title": {"x"}})
		if w.Code != 404 {
			t.Fatal("route access", route, w.Code)
		}
	}
	if err := a.store.LinkDiscussion(t.Context(), u[1], id, "javascript:alert(1)"); err == nil {
		t.Fatal("unsafe discussion")
	}
}

func TestGroupsWorkWithNativeFormsAndRevokedMembership(t *testing.T) {
	a, u := fixture(t)
	alice := login(t, a, "alice")
	w := alice.request("POST", "/groups", url.Values{"name": {"Quiet room"}, "member_" + strconv.FormatInt(u[1], 10): {"1"}})
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	groups, err := a.store.Groups(t.Context(), u[1])
	if err != nil || len(groups) != 1 {
		t.Fatal(groups, err)
	}
	gid := groups[0].ID
	w = alice.request("GET", "/groups", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), fmt.Sprintf(`name="member_%d" value="1" checked`, u[1])) {
		t.Fatal(w.Code, w.Body.String())
	}
	bobby := login(t, a, "bobby")
	w = bobby.request("POST", "/groups", url.Values{"id": {strconv.FormatInt(gid, 10)}, "name": {"Changed"}})
	if w.Code != 404 {
		t.Fatal("nonowner membership edit", w.Code)
	}
	w = alice.request("POST", "/groups", url.Values{"id": {strconv.FormatInt(gid, 10)}, "name": {"Quiet room"}})
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = bobby.request("GET", "/groups", nil)
	if strings.Contains(w.Body.String(), "Quiet room") {
		t.Fatal("removed group still disclosed")
	}
}
