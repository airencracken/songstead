// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/airencracken/songstead/internal/store"
)

func TestDiscussionLocationHTTPContractsAndHistory(t *testing.T) {
	a, users, owner := ownerBrowser(t)
	member, stranger := login(t, a, "bobby"), login(t, a, "carol")
	id, err := a.store.Recommend(t.Context(), users[0], users[1], "https://music.example/song", "Private recommendation provenance")
	if err != nil {
		t.Fatal(err)
	}
	path := "/recommendations/" + strconv.FormatInt(id, 10)
	if err := a.store.AddComment(t.Context(), users[1], id, "Earlier local conversation"); err != nil {
		t.Fatal(err)
	}
	if err := a.store.LinkDiscussion(t.Context(), users[1], id, "https://boards.example.org/topics/123"); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"songstead", "witmoot", "both"} {
		values := settingsValues()
		values.Set("discussion_mode", mode)
		if w := owner.request("POST", "/admin/settings", values); w.Code != 303 {
			t.Fatal(mode, w.Code, w.Body.String())
		}
		w := member.request("GET", path, nil)
		body := w.Body.String()
		if w.Code != 200 || !strings.Contains(body, "Earlier local conversation") || !strings.Contains(body, "https://boards.example.org/topics/123") {
			t.Fatal("existing history disappeared", mode, w.Code, body)
		}
		if strings.Contains(body, `>Add comment</button>`) != (mode != "witmoot") || strings.Contains(body, `>Prepare a Witmoot discussion</button>`) != (mode != "songstead") {
			t.Fatal("wrong discussion actions", mode, body)
		}
		if mode != "songstead" && (!strings.Contains(body, "separate Witmoot account") || !strings.Contains(body, "Ask your host for an invitation")) {
			t.Fatal("account requirement missing", mode)
		}
		w = member.request("POST", path+"/comments", url.Values{"body": {"A member without a Witmoot account"}})
		want := 303
		if mode == "witmoot" {
			want = http.StatusForbidden
		}
		if w.Code != want {
			t.Fatal("comment route bypass or inaccessible fallback", mode, w.Code, w.Body.String())
		}
		w = member.request("POST", path+"/share", nil)
		want = 200
		if mode == "songstead" {
			want = http.StatusForbidden
		}
		if w.Code != want || (mode != "songstead" && (strings.Contains(w.Body.String(), "Private recommendation provenance") || !strings.Contains(w.Body.String(), "A separate Witmoot account is required"))) {
			t.Fatal("handoff mode or privacy", mode, w.Code, w.Body.String())
		}
		// Policy cannot grant a member access to someone else's private discussion.
		for _, suffix := range []string{"", "/comments", "/share"} {
			method := "POST"
			if suffix == "" {
				method = "GET"
			}
			w = stranger.request(method, path+suffix, url.Values{"body": {"Intruder"}})
			if w.Code != 404 || strings.Contains(w.Body.String(), "Earlier local conversation") {
				t.Fatal("private route leaked", mode, suffix, w.Code)
			}
		}
		for _, suffix := range []string{"/comments", "/share"} {
			req := httptest.NewRequest("POST", path+suffix, strings.NewReader("body=No+token"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			for _, cookie := range member.cookies {
				req.AddCookie(cookie)
			}
			result := httptest.NewRecorder()
			a.ServeHTTP(result, req)
			if result.Code != 403 {
				t.Fatal("mode bypassed CSRF", mode, suffix, result.Code)
			}
		}
	}
	items, err := a.store.Comments(t.Context(), users[1], id)
	if err != nil || len(items) != 3 {
		t.Fatal("blocked comment persisted or old comments lost", len(items), err)
	}
	restarted, err := New(a.store, a.config)
	if err != nil {
		t.Fatal(err)
	}
	member.a = restarted
	if w := member.request("GET", path, nil); !strings.Contains(w.Body.String(), ">Add comment</button>") || !strings.Contains(w.Body.String(), ">Prepare a Witmoot discussion</button>") {
		t.Fatal("mode lost on restart")
	}
	// Disabling the handoff retains saved links, which remain per user.
	values := settingsValues()
	values.Set("discussion_mode", "songstead")
	owner.request("POST", "/admin/settings", values)
	if w := member.request("POST", path+"/discussion", url.Values{"remove": {"1"}, "url": {"https://boards.example.org/topics/123"}}); w.Code != 303 {
		t.Fatal("disabled integration prevented removing personal link", w.Code)
	}
}

func TestDiscussionLocationInvalidDraftAndOwnerPermission(t *testing.T) {
	a, _, owner := ownerBrowser(t)
	member := login(t, a, "alice")
	good := settingsValues()
	if w := owner.request("POST", "/admin/settings", good); w.Code != 303 {
		t.Fatal(w.Code)
	}
	for _, mode := range []string{"", "unknown", "<script>alert(1)</script>", "both'; DROP TABLE users;--"} {
		bad := settingsValues()
		bad.Set("discussion_mode", mode)
		bad.Set("name", "Draft name")
		w := owner.request("POST", "/admin/settings", bad)
		if w.Code != 422 || !strings.Contains(w.Body.String(), `value="Draft name"`) || strings.Contains(w.Body.String(), "<script>alert(1)</script>") {
			t.Fatal("invalid draft lost or accepted", mode, w.Code)
		}
		got, err := a.store.Settings(t.Context(), store.DefaultSettings("", ""))
		if err != nil || got.Name != "Friends" || got.DiscussionMode != "both" {
			t.Fatal("invalid draft partly persisted", got, err)
		}
	}
	for _, mode := range []string{"witmoot", "both"} {
		bad := settingsValues()
		bad.Set("discussion_mode", mode)
		bad.Set("witmoot_url", "")
		if w := owner.request("POST", "/admin/settings", bad); w.Code != 422 || !strings.Contains(w.Body.String(), `value="`+mode+`" selected`) {
			t.Fatal("missing connection accepted or draft reset", mode, w.Code)
		}
	}
	duplicate := settingsValues()
	duplicate["discussion_mode"] = []string{"songstead", "witmoot"}
	if w := owner.request("POST", "/admin/settings", duplicate); w.Code != 400 {
		t.Fatal("ambiguous policy accepted", w.Code)
	}
	if w := member.request("POST", "/admin/settings", good); w.Code != 403 {
		t.Fatal("member changed discussion location", w.Code)
	}
	legacy := settingsValues()
	legacy.Del("discussion_mode")
	if w := owner.request("POST", "/admin/settings", legacy); w.Code != 303 {
		t.Fatal("older form rejected", w.Code)
	}
	got, err := a.store.Settings(t.Context(), store.DefaultSettings("", ""))
	if err != nil || got.DiscussionMode != "both" {
		t.Fatal("older form changed policy", got, err)
	}
}
