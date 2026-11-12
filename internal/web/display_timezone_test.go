package web

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDisplayTimezoneAccountRoutesAndRendering(t *testing.T) {
	a, users, _ := ownerBrowser(t)
	c := login(t, a, "alice")
	if w := c.request("GET", "/account", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `value="UTC"`) {
		t.Fatal("missing default preference", w.Code)
	}
	if w := c.request("POST", "/account/timezone", url.Values{"timezone": {"America/Los_Angeles"}, "user_id": {fmt.Sprint(users[1])}}); w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	if zone, err := a.store.Timezone(t.Context(), users[0]); err != nil || zone != "America/Los_Angeles" {
		t.Fatal(zone, err)
	}
	if zone, _ := a.store.Timezone(t.Context(), users[1]); zone != "UTC" {
		t.Fatal("changed another account")
	}
	id, err := a.store.Recommend(t.Context(), users[0], users[1], "https://example.org/music", "timezone fixture")
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 1, 1, 0, 15, 0, 0, time.UTC)
	if _, err = a.store.DB().Exec("UPDATE recommendations SET created_at=? WHERE id=?", stamp.Unix(), id); err != nil {
		t.Fatal(err)
	}
	w := c.request("GET", fmt.Sprintf("/recommendations/%d", id), nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Dec 31, 2025 · 16:15 PST") || !strings.Contains(w.Body.String(), `datetime="2026-01-01T00:15:00Z"`) {
		t.Fatal("local/UTC date disagreement", w.Code, w.Body.String())
	}
	for _, values := range []url.Values{{"timezone": {"Local"}}, {"timezone": {"UTC", "Asia/Tokyo"}}, {"timezone": {"<script>bad</script>"}}} {
		w := c.request("POST", "/account/timezone", values)
		want := 422
		if len(values["timezone"]) > 1 {
			want = 400
		}
		if w.Code != want || strings.Contains(w.Body.String(), "<script>bad</script>") {
			t.Fatal("invalid preference accepted or unescaped", w.Code)
		}
		if zone, _ := a.store.Timezone(t.Context(), users[0]); zone != "America/Los_Angeles" {
			t.Fatal("invalid save changed preference")
		}
	}
	if _, err = a.store.DB().Exec("UPDATE users SET suspended=1 WHERE id=?", users[0]); err != nil {
		t.Fatal(err)
	}
	if a.store.SetTimezone(t.Context(), users[0], "UTC") == nil {
		t.Fatal("suspended account changed preference")
	}
}
