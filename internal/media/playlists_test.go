// SPDX-License-Identifier: AGPL-3.0-or-later
package media

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

const albumPlaylist = "OLAK5uy_mEf5zzrxJlXov2xQ1Ph7o0cgPF7XfSJ00"

func TestYouTubePlaylistMetadata(t *testing.T) {
	raw := "https://youtube.com/playlist?list=" + albumPlaylist + "&si=F4sY7bdDif0VicLu"
	key, kind, err := Identity(raw)
	if err != nil || key != "youtube:playlist:"+albumPlaylist || kind != "album" || !SupportsMetadata(raw) {
		t.Fatal("album playlist not recognized", key, kind, err)
	}
	calls := 0
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "www.youtube.com" || r.URL.Path != "/oembed" || r.URL.Query().Get("url") != "https://www.youtube.com/playlist?list="+albumPlaylist {
			t.Fatal("unexpected playlist destination", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"title":"Netherworlds","thumbnail_url":"https://attacker.example/art.png","html":"<script>ignored</script>"}`))}, nil
	})}
	meta, err := Fetch(t.Context(), client, raw)
	if err != nil || meta.Title != "Netherworlds" || meta.Thumbnail != "" || calls != 1 {
		t.Fatal(meta, err, calls)
	}
	for _, host := range []string{"www.youtube.com", "music.youtube.com", "m.youtube.com"} {
		other, _, _ := Identity("https://" + host + "/playlist?list=" + albumPlaylist)
		if other != key {
			t.Fatal("playlist variants differ", other)
		}
	}
	track, kind, _ := Identity("https://youtube.com/watch?v=kwZzvUigREQ&list=" + albumPlaylist)
	if track != "youtube:kwZzvUigREQ" || kind != "track" {
		t.Fatal("track confused with playlist", track, kind)
	}
	for _, raw := range []string{"https://youtube.com.attacker.example/playlist?list=" + albumPlaylist, "https://youtube.com/playlist?list=bad!", "https://youtube.com/playlist", "https://youtube.com/playlist?list=" + albumPlaylist + "&list=anotherplaylist"} {
		if SupportsMetadata(raw) {
			t.Fatal("unsafe playlist supported", raw)
		}
	}
}

func TestYouTubePlaylistsKeepKindsAndFailurePaths(t *testing.T) {
	for _, tt := range []struct{ id, kind string }{{albumPlaylist, "album"}, {"PL1234567890abcdef", "link"}} {
		raw := "https://www.youtube.com/playlist?list=" + tt.id
		_, kind, err := Identity(raw)
		if err != nil || kind != tt.kind {
			t.Fatal(kind, err)
		}
		for _, status := range []int{http.StatusNotFound, http.StatusForbidden} {
			calls := 0
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
			})}
			if _, err := Fetch(t.Context(), client, raw); err == nil || calls != 1 {
				t.Fatal("failed playlist invented video artwork", err, calls)
			}
		}
	}
}
