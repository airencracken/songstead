// SPDX-License-Identifier: AGPL-3.0-or-later

// Package media validates links without fetching them and retrieves optional metadata.
package media

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Link struct {
	Original, Provider, Title, Type, VideoID string
}

var videoID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// Parse preserves the submitted URL. Unsupported providers are valid links.
func Parse(raw string) (Link, error) {
	link := Link{Original: raw, Provider: "unknown", Title: raw, Type: "url"}
	if len(raw) > 4096 || !utf8.ValidString(raw) || strings.IndexFunc(raw, unicode.IsSpace) >= 0 || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return link, errors.New("use a complete HTTP or HTTPS URL without spaces")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Opaque != "" {
		return link, errors.New("use a complete HTTP or HTTPS URL without credentials")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return link, errors.New("URL port must be between 1 and 65535")
		}
	}
	host := strings.ToLower(u.Hostname())
	for domain, provider := range map[string]string{
		"youtube.com": "youtube", "youtu.be": "youtube", "spotify.com": "spotify",
		"bandcamp.com": "bandcamp", "soundcloud.com": "soundcloud", "music.apple.com": "apple", "pandora.com": "pandora",
	} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			link.Provider = provider
			break
		}
	}
	if host == "youtu.be" {
		link.VideoID = strings.TrimPrefix(u.Path, "/")
	}
	if host == "youtube.com" || host == "www.youtube.com" || host == "m.youtube.com" || host == "music.youtube.com" {
		if u.Path == "/watch" {
			link.VideoID = u.Query().Get("v")
		}
		for _, prefix := range []string{"/shorts/", "/live/", "/embed/"} {
			if strings.HasPrefix(u.Path, prefix) {
				link.VideoID = strings.TrimPrefix(u.Path, prefix)
			}
		}
	}
	if !videoID.MatchString(link.VideoID) {
		link.VideoID = ""
	}
	if link.VideoID != "" {
		link.Type = "video"
	}
	return link, nil
}
