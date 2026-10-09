// SPDX-License-Identifier: AGPL-3.0-or-later
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/quick"

	"github.com/airencracken/comfylib/brandimage"
)

func TestProviderMetadataAndArtworkContracts(t *testing.T) {
	var encoded bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{R: 120, A: 255})
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ raw, host, source, thumbnail string }{
		{"https://youtu.be/dQw4w9WgXcQ?t=30", "www.youtube.com", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg"},
		{"https://open.spotify.com/intl-de/album/0sNOF9WDwhWunNAHPD3Baj?si=private", "open.spotify.com", "https://open.spotify.com/album/0sNOF9WDwhWunNAHPD3Baj", "https://i.scdn.co/image/ab67656300005f1ff8141e891abf749375772343"},
		{"https://soundcloud.com/forss/flickermood?utm_source=private", "soundcloud.com", "https://soundcloud.com/forss/flickermood", "https://i1.sndcdn.com/artworks-test-large.jpg"},
	} {
		t.Run(tt.host, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
					t.Fatal("unexpected provider credentials", r)
				}
				if calls == 1 {
					if r.URL.Host != tt.host || r.URL.Path != "/oembed" || r.URL.Query().Get("url") != tt.source {
						t.Fatal("noncanonical provider endpoint", r.URL)
					}
					data, _ := json.Marshal(map[string]string{"title": "A recording", "author_name": "An artist", "thumbnail_url": tt.thumbnail, "html": "<script>ignored</script>"})
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
				}
				if calls != 2 || r.URL.String() != tt.thumbnail {
					t.Fatal("untrusted image request", r.URL)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(encoded.Bytes()))}, nil
			})}
			meta, err := Fetch(t.Context(), client, tt.raw)
			if err != nil || meta.Title != "A recording" || meta.Artist != "An artist" || meta.Thumbnail != tt.thumbnail || calls != 2 {
				t.Fatal(meta, err, calls)
			}
			config, format, err := image.DecodeConfig(bytes.NewReader(meta.Artwork))
			if err != nil || format != "png" || config.Width != 4 || config.Height != 4 {
				t.Fatal("invalid cached image", config, format, err)
			}
		})
	}
}

func TestThumbnailDestinationsAndBounds(t *testing.T) {
	for _, raw := range []string{"https://localhost/art.png", "http://i.scdn.co/image/key", "https://i.scdn.co.attacker.example/image/key", "https://user@i.scdn.co/image/key", "https://i.scdn.co:443/image/key", "https://i.scdn.co/image/key?secret=1", "https://i.scdn.co/image/../key", "https://i.scdn.co/image/a%2fb", "https://i.scdn.co/image/a#b", "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg"} {
		if thumbnailURL(raw, "spotify") {
			t.Fatal("unsafe image accepted", raw)
		}
		calls := 0
		client := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
			calls++
			data, _ := json.Marshal(map[string]string{"title": "Safe title", "thumbnail_url": raw})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
		})}
		meta, err := Fetch(t.Context(), client, "https://open.spotify.com/track/0sNOF9WDwhWunNAHPD3Baj")
		if err != nil || meta.Thumbnail != "" || len(meta.Artwork) != 0 || calls != 1 {
			t.Fatal("unsafe URL fetched", raw, meta, err, calls)
		}
	}
	for _, tt := range []struct {
		name, body string
		status     int
		offline    bool
	}{
		{"invalid image", "<svg><script>bad</script></svg>", 200, false},
		{"oversized", strings.Repeat("x", brandimage.MaxBytes+1), 200, false},
		{"redirect", "", 302, false}, {"missing", "", 404, false}, {"offline", "", 200, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/oembed" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"title":"Keep this text"}`))}, nil
				}
				if tt.offline {
					return nil, errors.New("offline")
				}
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			})}
			meta, err := Fetch(t.Context(), client, "https://youtu.be/dQw4w9WgXcQ")
			if err != nil || meta.Title != "Keep this text" || meta.Thumbnail == "" || len(meta.Artwork) != 0 {
				t.Fatal("image failure discarded metadata", meta, err)
			}
		})
	}
}

func TestUnsupportedLinksNeverFetch(t *testing.T) {
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		t.Fatal("unsupported URL fetched", r.URL)
		return nil, nil
	})}
	for _, raw := range []string{"https://music.example/track", "https://attacker.spotify.com/track/0sNOF9WDwhWunNAHPD3Baj", "https://open.spotify.com:8443/track/0sNOF9WDwhWunNAHPD3Baj", "http://open.spotify.com/track/0sNOF9WDwhWunNAHPD3Baj", "https://soundcloud.com/oembed", "https://soundcloud.com/user/../../secret", "https://soundcloud.com/user/%2fsecret", "https://localhost/song"} {
		if SupportsMetadata(raw) {
			t.Fatal("unsupported metadata shape", raw)
		}
		if _, err := Fetch(context.Background(), client, raw); err == nil {
			t.Fatal("unsupported accepted", raw)
		}
	}
	if err := quick.Check(func(raw string) bool {
		if !thumbnailURL(raw, "spotify") {
			return true
		}
		return strings.HasPrefix(raw, "https://i.scdn.co/image/") && !strings.ContainsAny(raw, "?#@%")
	}, nil); err != nil {
		t.Fatal(err)
	}
}
