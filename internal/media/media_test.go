// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
)

func TestURLContract(t *testing.T) {
	for raw, provider := range map[string]string{
		"https://youtu.be/dQw4w9WgXcQ?t=30":                         "youtube",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=anything": "youtube",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ":                "youtube",
		"https://open.spotify.com/album/example":                    "spotify",
		"https://artist.bandcamp.com/album/example":                 "bandcamp",
		"https://soundcloud.com/artist/track":                       "soundcloud",
		"https://music.apple.com/us/album/example":                  "apple",
		"https://pandora.com/song/example":                          "pandora",
		"https://example.org:8080/a?q=one%20two#anchor":             "unknown",
		"https://youtube.com.attacker.example/watch?v=dQw4w9WgXcQ":  "unknown",
		"http://127.0.0.1/song":                                     "unknown", // Stored as a link, never fetched.
	} {
		link, err := Parse(raw)
		if err != nil || link.Original != raw || link.Provider != provider {
			t.Fatalf("%q: %+v %v", raw, link, err)
		}
	}
	for _, raw := range []string{"", "relative/path", "javascript:alert(1)", "file:///etc/passwd", "ftp://example.org/file", "https://user:password@example.org/song", "https://example.org/has space", "https://example.org/\x00", "https://example.org/\xff", "https://example.org:0/song", "https://example.org:65536/song", "https://example.org:bad/song", strings.Repeat("x", 4097)} {
		if _, err := Parse(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	for _, raw := range []string{"https://www.youtube.com/watch?v=%22%3E%3Cscript%3E", "https://sub.youtube.com/watch?v=dQw4w9WgXcQ", "https://youtu.be/dQw4w9WgXcQ/extra"} {
		link, err := Parse(raw)
		if err != nil || link.VideoID != "" {
			t.Fatalf("unsafe embed: %+v %v", link, err)
		}
	}
}

func TestSSRFAddressPolicy(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254", "0.0.0.0", "100.64.0.1", "192.0.0.1", "192.0.2.1", "198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "255.255.255.255", "::1", "::", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "fec0::1", "2001:db8::1", "2002:7f00:1::1", "64:ff9b::7f00:1", "2001::1"} {
		if publicAddress(netip.MustParseAddr(raw)) {
			t.Fatal("non-public address accepted", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111", "2001:4860:4860::8888"} {
		if !publicAddress(netip.MustParseAddr(raw)) {
			t.Fatal("public address refused", raw)
		}
	}
}

func TestDNSAnswersAreCheckedAndPinned(t *testing.T) {
	ctx := context.Background()
	dials := 0
	dial := func(_ context.Context, _ string, address string) (net.Conn, error) {
		dials++
		if address != "8.8.8.8:443" {
			t.Fatal("hostname resolved twice or wrong address", address)
		}
		a, b := net.Pipe()
		b.Close()
		return a, nil
	}
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	conn, err := checkedDial(ctx, "tcp", "www.youtube.com:443", lookup, dial)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if dials != 1 {
		t.Fatal(dials)
	}
	for _, records := range [][]netip.Addr{nil, {netip.MustParseAddr("127.0.0.1")}, {netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("169.254.169.254")}} {
		before := dials
		_, err := checkedDial(ctx, "tcp", "www.youtube.com:443", func(context.Context, string, string) ([]netip.Addr, error) { return records, nil }, dial)
		if err == nil || dials != before {
			t.Fatal("unsafe DNS caused a dial", records, err)
		}
	}
	for _, destination := range []string{"localhost:443", "www.youtube.com:80", "www.youtube.com.attacker.example:443", "127.0.0.1:443", "bad"} {
		if _, err := checkedDial(ctx, "tcp", destination, lookup, dial); err == nil {
			t.Fatal("destination accepted", destination)
		}
	}
	if _, err := checkedDial(ctx, "tcp", "www.youtube.com:443", func(context.Context, string, string) ([]netip.Addr, error) { return nil, errors.New("DNS offline") }, dial); err == nil {
		t.Fatal("DNS error ignored")
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestYouTubeMetadataContracts(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
		valid      bool
	}{
		{"valid", `{"title":"A song","author_name":"Artist","html":"<script>bad</script>","thumbnail_url":"http://localhost/secret"}`, 200, true},
		{"missing", `{}`, 200, false}, {"broken", `not JSON`, 200, false},
		{"deleted", `{}`, 404, false}, {"redirect", `{}`, 302, false},
		{"oversized", strings.Repeat("x", 65537), 200, false},
		{"control", `{"title":"song\u0000"}`, 200, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "www.youtube.com" || r.URL.Path != "/oembed" || r.URL.Query().Get("url") != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
					t.Fatal("wrong metadata request", r.URL)
				}
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body)), Header: make(http.Header)}, nil
			})}
			got, err := YouTube(context.Background(), client, "dQw4w9WgXcQ")
			if (err == nil) != tt.valid {
				t.Fatal(got, err)
			}
			if tt.valid && (got.Title != "A song" || got.Thumbnail != "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg") {
				t.Fatal("untrusted artwork or text", got)
			}
		})
	}
	client := Client()
	if client.Timeout <= 0 || client.CheckRedirect(nil, nil) != http.ErrUseLastResponse || client.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("unbounded or redirected/proxied metadata client")
	}
	if _, err := YouTube(context.Background(), client, "https://localhost/"); err == nil {
		t.Fatal("invalid id accepted")
	}
}

func FuzzURLParsing(f *testing.F) {
	for _, seed := range []string{"https://youtu.be/dQw4w9WgXcQ", "https://example.org/song", "javascript:alert(1)", "https://user@host/", "\x00"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		link, err := Parse(raw)
		if err == nil && (link.Original != raw || link.VideoID != "" && !videoID.MatchString(link.VideoID)) {
			t.Fatal("parser changed URL or accepted unsafe embed")
		}
	})
}
