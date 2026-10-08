// SPDX-License-Identifier: AGPL-3.0-or-later
package media

import "testing"

func TestConservativeMusicIdentities(t *testing.T) {
	a, kind, err := Identity("https://youtu.be/dQw4w9WgXcQ?t=10")
	if err != nil || kind != "track" {
		t.Fatal(a, kind, err)
	}
	b, _, _ := Identity("https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=another")
	if a != b {
		t.Fatal("video variants differ", a, b)
	}
	for _, kind := range []string{"track", "album", "artist"} {
		raw := "https://open.spotify.com/" + kind + "/0123456789abcdefghijkL?si=share"
		a, k, err := Identity(raw)
		b, _, _ := Identity("https://open.spotify.com/" + kind + "/0123456789abcdefghijkL")
		if err != nil || k != kind || a != b {
			t.Fatal(a, b, k, err)
		}
	}
	for _, raw := range []string{"https://open.spotify.com/track/0123456789abcdefghijkL/extra", "https://open.spotify.com/track/0123456789abcdefghijk!"} {
		key, kind, err := Identity(raw)
		if err != nil || kind != "link" || key == "spotify:track:0123456789abcdefghijkL" {
			t.Fatal("invalid provider path merged", key, kind, err)
		}
	}
	a, _, _ = Identity("https://music.example/recording?edition=a#fragment")
	b, _, _ = Identity("https://music.example/recording?edition=b")
	if a == b {
		t.Fatal("unknown URL queries merged")
	}
	b, _, _ = Identity("https://music.example/recording?edition=a")
	if a != b {
		t.Fatal("fragment prevented identity")
	}
}
