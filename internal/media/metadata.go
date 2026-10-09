// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/airencracken/comfylib/brandimage"
)

type Metadata struct {
	Title, Artist, Thumbnail string
	Artwork                  []byte
}

// Keep the destination list finite: neither submitted links nor provider HTML
// may decide which servers the worker can contact.
func metadataHost(host string) bool {
	switch host {
	case "www.youtube.com", "open.spotify.com", "soundcloud.com", "i.ytimg.com", "i.scdn.co", "image-cdn-ak.spotifycdn.com", "image-cdn-fa.spotifycdn.com", "i1.sndcdn.com":
		return true
	}
	return false
}

var blocked = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("2002::/16"),
}

func publicAddress(address netip.Addr) bool {
	address = address.Unmap()
	if address.Is6() && !netip.MustParsePrefix("2000::/3").Contains(address) {
		return false
	}
	if address.Zone() != "" || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range blocked {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

// safeDial validates every DNS answer, then connects to the verified IP rather
// than resolving the hostname again. TLS still verifies the original hostname.
func safeDial(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: 3 * time.Second}
	return checkedDial(ctx, network, address, net.DefaultResolver.LookupNetIP, dialer.DialContext)
}

func checkedDial(ctx context.Context, network, address string, lookup func(context.Context, string, string) ([]netip.Addr, error), dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || !metadataHost(host) || port != "443" {
		return nil, errors.New("metadata destination refused")
	}
	addresses, err := lookup(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, errors.New("no metadata address")
	}
	for _, ip := range addresses {
		if !publicAddress(ip) {
			return nil, errors.New("non-public metadata address")
		}
	}
	var last error
	for _, ip := range addresses {
		conn, err := dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}

func Client() *http.Client {
	return &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{Proxy: nil, DialContext: safeDial, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 3 * time.Second, MaxConnsPerHost: 2, MaxIdleConns: 2},
	}
}

func YouTube(ctx context.Context, client *http.Client, id string) (Metadata, error) {
	if !videoID.MatchString(id) {
		return Metadata{}, errors.New("invalid YouTube video ID")
	}
	q := url.Values{"url": {"https://www.youtube.com/watch?v=" + id}, "format": {"json"}}
	meta, err := oembed(ctx, client, "https://www.youtube.com/oembed?"+q.Encode())
	if err != nil {
		return Metadata{}, err
	}
	// Provider HTML and arbitrary thumbnail URLs are ignored for YouTube.
	meta.Thumbnail = "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg"
	return meta, nil
}

func oembed(ctx context.Context, client *http.Client, endpoint string) (Metadata, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return Metadata{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Metadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Metadata{}, fmt.Errorf("metadata status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(data) > 65536 || !utf8.Valid(data) {
		return Metadata{}, errors.New("metadata response too large or unreadable")
	}
	var result struct {
		Title     string `json:"title"`
		Author    string `json:"author_name"`
		Thumbnail string `json:"thumbnail_url"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return Metadata{}, err
	}
	if !metadataText(result.Title, 500) || result.Title == "" || !metadataText(result.Author, 200) {
		return Metadata{}, errors.New("invalid metadata text")
	}
	return Metadata{Title: result.Title, Artist: result.Author, Thumbnail: result.Thumbnail}, nil
}

// metadataEndpoint canonicalizes provider identities rather than fetching the
// submitted address. Unsupported links remain useful without network access.
func metadataEndpoint(raw string) (endpoint, provider, id string) {
	link, err := Parse(raw)
	if err != nil {
		return
	}
	if link.VideoID != "" {
		return "", "youtube", link.VideoID
	}
	u, _ := url.Parse(raw)
	if u.Scheme != "https" || u.Port() != "" {
		return
	}
	key, _, _ := Identity(raw)
	if u.Hostname() == "open.spotify.com" && strings.HasPrefix(key, "spotify:") {
		parts := strings.Split(key, ":")
		return "https://open.spotify.com/oembed?" + url.Values{"url": {"https://open.spotify.com/" + parts[1] + "/" + parts[2]}}.Encode(), "spotify", ""
	}
	if u.Hostname() == "soundcloud.com" || u.Hostname() == "www.soundcloud.com" {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) < 2 || len(parts) > 3 || (len(parts) == 3 && parts[1] != "sets") {
			return
		}
		for _, part := range parts {
			if part == "" || strings.IndexFunc(part, func(r rune) bool {
				return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
			}) >= 0 {
				return
			}
		}
		return "https://soundcloud.com/oembed?" + url.Values{"url": {"https://soundcloud.com/" + strings.Join(parts, "/")}, "format": {"json"}}.Encode(), "soundcloud", ""
	}
	return
}

func SupportsMetadata(raw string) bool {
	_, provider, _ := metadataEndpoint(raw)
	return provider != ""
}

func thumbnailURL(raw, provider string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return false
	}
	switch provider {
	case "youtube":
		parts := strings.Split(u.Path, "/")
		return u.Host == "i.ytimg.com" && len(parts) == 4 && parts[1] == "vi" && videoID.MatchString(parts[2]) && parts[3] == "hqdefault.jpg"
	case "spotify":
		return (u.Host == "i.scdn.co" || u.Host == "image-cdn-ak.spotifycdn.com" || u.Host == "image-cdn-fa.spotifycdn.com") && strings.HasPrefix(u.Path, "/image/") && imageKey(strings.TrimPrefix(u.Path, "/image/"))
	case "soundcloud":
		return u.Host == "i1.sndcdn.com" && strings.HasPrefix(u.Path, "/artworks-") && imageKey(strings.TrimPrefix(u.Path, "/"))
	}
	return false
}

func imageKey(key string) bool {
	return len(key) > 0 && len(key) <= 200 && strings.IndexFunc(key, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.')
	}) < 0 && !strings.Contains(key, "..")
}

// Fetch retrieves bounded text and caches artwork as a local PNG, so viewing
// music never makes a thumbnail request from the listener's browser to a provider.
// Artwork failures retain text; the durable worker retries missing images.
func Fetch(ctx context.Context, client *http.Client, raw string) (Metadata, error) {
	endpoint, provider, id := metadataEndpoint(raw)
	if provider == "" {
		return Metadata{}, errors.New("unsupported metadata provider")
	}
	var meta Metadata
	var err error
	if provider == "youtube" {
		meta, err = YouTube(ctx, client, id)
	} else {
		meta, err = oembed(ctx, client, endpoint)
	}
	if err != nil {
		if provider != "youtube" {
			return Metadata{}, err
		}
		// Artwork does not depend on oEmbed availability (for example when a
		// video's title is unavailable from the server's region).
		meta = Metadata{Title: raw, Thumbnail: "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg"}
	}
	if !thumbnailURL(meta.Thumbnail, provider) {
		meta.Thumbnail = ""
		return meta, nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", meta.Thumbnail, nil)
	if err != nil {
		return meta, nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return meta, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return meta, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, brandimage.MaxBytes+1))
	if err != nil {
		return meta, nil
	}
	art, err := brandimage.Normalize(data)
	if err == nil && len(art) <= 4<<20 {
		meta.Artwork = art
	}
	return meta, nil
}

func metadataText(s string, max int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && strings.IndexFunc(s, unicode.IsControl) < 0
}
