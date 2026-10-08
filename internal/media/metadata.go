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
)

type Metadata struct{ Title, Artist, Thumbnail string }

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
	if err != nil || host != "www.youtube.com" || port != "443" {
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
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.youtube.com/oembed?"+q.Encode(), nil)
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
	if err != nil || len(data) > 65536 {
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
	// Provider HTML is intentionally ignored. Generate artwork from the verified ID.
	return Metadata{result.Title, result.Author, "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg"}, nil
}

func metadataText(s string, max int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && strings.IndexFunc(s, unicode.IsControl) < 0
}
