// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/airencracken/songstead/internal/store"
)

func TestRecentCommentsRoutesPrivacySpoilersAndPagination(t *testing.T) {
	a, u := fixture(t)
	ctx := t.Context()
	alice := login(t, a, "alice")
	bobby := login(t, a, "bobby")
	carol := login(t, a, "carol")
	shared, err := a.store.ShareMusic(ctx, u[0], "https://music.example/shared", "Shared note", "track", "Shared music", "Demo artist")
	if err != nil {
		t.Fatal(err)
	}
	private, err := a.store.RecommendMusic(ctx, u[0], u[1], 0, "https://music.example/shared", "SECRET gift", "track", "Shared music", "Demo artist")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.AddComment(ctx, u[0], private, "SECRET private comment"); err != nil {
		t.Fatal(err)
	}
	if err := a.store.React(ctx, u[1], shared, store.Reaction{Listening: "listened", Rating: 1, Note: "SECRET listening notes"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 27; i++ {
		if err := a.store.AddComment(ctx, u[1], shared, fmt.Sprintf("Visible reaction %02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.store.AddComment(ctx, u[0], shared, "SECRET timestamp reaction at 0:40"); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetAnnotationMode(ctx, u[2], "hidden"); err != nil {
		t.Fatal(err)
	}
	w := carol.request("GET", "/recent/comments?reveal=1", nil)
	body := w.Body.String()
	if w.Code != 200 || strings.Contains(body, "SECRET") || strings.Count(body, `class="comment-activity"`) != 25 || !strings.Contains(body, "Visible reaction 26") || strings.Contains(body, "Visible reaction 01") {
		t.Fatal("feed privacy or first page", w.Code, body)
	}
	for _, text := range []string{`href="/recent" aria-current="page"`, `href="/recent/comments" aria-current="page"`, `/users/` + strconv.FormatInt(u[1], 10) + `/picture`, `Shared music`, `Demo artist`, `/account#spoiler-preferences`} {
		if !strings.Contains(body, text) {
			t.Fatal("missing feed UI", text)
		}
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("feed is cached")
	}
	older := regexp.MustCompile(`href="(/recent/comments\?before=[0-9]+)"`).FindStringSubmatch(body)
	if len(older) != 2 {
		t.Fatal("missing older page")
	}
	if err := a.store.AddComment(ctx, u[0], shared, "Newer reaction between page requests"); err != nil {
		t.Fatal(err)
	}
	w = carol.request("GET", older[1], nil)
	body = w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "Visible reaction 01") || !strings.Contains(body, "Visible reaction 00") || strings.Contains(body, "between page") || strings.Contains(body, `rel="next"`) || !strings.Contains(body, ">Newest comments</a>") {
		t.Fatal("unstable older page", w.Code, body)
	}
	w = bobby.request("GET", "/recent", nil)
	if !strings.Contains(w.Body.String(), `href="/recent/comments"`) {
		t.Fatal("feed not discoverable from Recent")
	}
	for _, query := range []string{"before=", "before=-1", "before=0", "before=abc", "before=9223372036854775808", "before=1&before=2", "before=%27OR%201=1", "before=%zz"} {
		if w := carol.request("GET", "/recent/comments?"+query, nil); w.Code != 400 || strings.Contains(w.Body.String(), "Visible reaction") {
			t.Fatal("invalid cursor", query, w.Code)
		}
	}
	if w := carol.request("HEAD", "/recent/comments", nil); w.Code != 200 {
		t.Fatal("HEAD route", w.Code)
	}
	if w := carol.request("POST", "/recent/comments", nil); w.Code != 405 {
		t.Fatal("method restriction", w.Code)
	}
	guest := httptest.NewRecorder()
	a.ServeHTTP(guest, httptest.NewRequest(http.MethodGet, "/recent/comments", nil))
	if guest.Code != 303 || strings.Contains(guest.Body.String(), "Visible reaction") {
		t.Fatal("anonymous feed")
	}
	if err := a.store.SetAnnotationMode(ctx, u[2], "spoiler-free"); err != nil {
		t.Fatal(err)
	}
	tracks, err := a.store.Recordings(ctx, u[2], shared)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetPosition(ctx, u[2], shared, tracks[0].ID, 40); err != nil {
		t.Fatal(err)
	}
	body = carol.request("GET", "/recent/comments", nil).Body.String()
	if !strings.Contains(body, "SECRET timestamp reaction") || strings.Contains(body, "SECRET private comment") || strings.Contains(body, "SECRET listening notes") {
		t.Fatal("position or private feedback boundary")
	}
	comments, err := a.store.Comments(ctx, u[2], shared)
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/recommendations/%d#comment-%d", shared, comments[len(comments)-1].ID)
	if !strings.Contains(body, `href="`+path+`">View comment &amp; conversation</a>`) {
		t.Fatal("missing exact comment link")
	}
	detail := carol.request("GET", path, nil).Body.String()
	if !strings.Contains(detail, fmt.Sprintf(`id="comment-%d"`, comments[len(comments)-1].ID)) {
		t.Fatal("anchor target absent")
	}
	if err := a.store.AddComment(ctx, u[0], shared, `<script>alert("reaction")</script>`); err != nil {
		t.Fatal(err)
	}
	body = alice.request("GET", "/recent/comments", nil).Body.String()
	if strings.Contains(body, `<script>alert`) || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatal("unescaped comment")
	}
	// No feed request changes account preferences or posts a comment.
	mode, err := a.store.AnnotationMode(ctx, u[2])
	if err != nil || mode != "spoiler-free" {
		t.Fatal("feed mutated preference")
	}
	if err := a.store.SetRole(ctx, "alice", "owner"); err != nil {
		t.Fatal(err)
	}
	settings := store.DefaultSettings("https://songs.example", "https://boards.example")
	settings.DiscussionMode = "witmoot"
	if err := a.store.SaveSettings(ctx, u[0], &settings); err != nil {
		t.Fatal(err)
	}
	body = carol.request("GET", "/recent/comments", nil).Body.String()
	if !strings.Contains(body, "Earlier comments here remain readable") || !strings.Contains(body, "Visible reaction 26") {
		t.Fatal("old feed disappeared in Witmoot mode")
	}
	if err := a.store.ChangeAccount(ctx, u[0], u[2], "suspended", "1"); err != nil {
		t.Fatal(err)
	}
	if w := carol.request("GET", "/recent/comments", nil); w.Code != 303 || strings.Contains(w.Body.String(), "Visible reaction") {
		t.Fatal("suspended session read feed", w.Code)
	}
}
