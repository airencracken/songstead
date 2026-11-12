// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"fmt"
	"github.com/airencracken/comfylib/suite"
	"net/url"
	"strings"
	"testing"
)

func TestDiscussionReturnRoutesKeepAuthorizationAndConfirmation(t *testing.T) {
	a, u := fixture(t)
	a.config.BaseURL = "https://music.example"
	a.config.WitmootURL = "https://board.example"
	id, e := a.store.ShareMusic(t.Context(), u[0], "https://provider.example/song", "A thought", "track", "A song", "")
	if e != nil {
		t.Fatal(e)
	}
	b := login(t, a, "alice")
	path := fmt.Sprintf("/recommendations/%d", id)
	link, e := suite.PrepareDiscussion("https://board.example/share", a.config.BaseURL+path, b.cookies["songstead_session"].Value)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := url.Parse(link)
	callback, _ := url.Parse(raw.Query().Get("return_to"))
	query := callback.Query()
	query.Set("thread", "https://board.example/topics/42")
	callback.RawQuery = query.Encode()
	response := b.request("GET", callback.RequestURI(), nil)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "Keep this discussion link?") {
		t.Fatal(response.Code, response.Body.String())
	}
	links, e := a.store.Discussions(t.Context(), u[0], id)
	if e != nil || len(links) != 0 {
		t.Fatal("GET saved a discussion", links, e)
	}
	other := login(t, a, "bobby")
	if response = other.request("GET", callback.RequestURI(), nil); response.Code != 400 {
		t.Fatal("another local session accepted return", response.Code)
	}
	form := url.Values{"url": {"https://evil.example/topics/42"}, "discussion_state": {query.Get("state")}}
	if response = b.request("POST", path+"/discussion", form); response.Code != 400 {
		t.Fatal("substituted discussion accepted", response.Code)
	}
	form.Set("url", "https://board.example/topics/42")
	if response = b.request("POST", path+"/discussion", form); response.Code != 303 {
		t.Fatal(response.Code, response.Body.String())
	}
	links, e = a.store.Discussions(t.Context(), u[0], id)
	if e != nil || len(links) != 1 {
		t.Fatal("POST did not save", links, e)
	}
	if response = b.request("GET", callback.RequestURI(), nil); response.Code != 400 {
		t.Fatal("used return accepted", response.Code)
	}
}
