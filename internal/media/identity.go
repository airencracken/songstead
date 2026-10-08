// SPDX-License-Identifier: AGPL-3.0-or-later
package media

import (
	"net/url"
	"regexp"
	"strings"
)

var spotifyIdentity = regexp.MustCompile(`^[A-Za-z0-9]{22}$`)

// Identity merges only known provider identities or exact normalized web URLs.
// Cross-provider recordings remain distinct until a person identifies them.
func Identity(raw string) (string, string, error) {
	link, err := Parse(raw)
	if err != nil {
		return "", "", err
	}
	if link.VideoID != "" {
		return "youtube:" + link.VideoID, "track", nil
	}
	u, _ := url.Parse(link.Original)
	u.Fragment = ""
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	kind := "link"
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if link.Provider == "Spotify" || link.Provider == "spotify" {
		for i, p := range parts {
			if (p == "track" || p == "album" || p == "artist") && i+2 == len(parts) && (i == 0 || i == 1 && strings.HasPrefix(parts[0], "intl-")) && spotifyIdentity.MatchString(parts[i+1]) {
				return "spotify:" + p + ":" + parts[i+1], p, nil
			}
		}
	}
	if link.Type == "song" || link.Type == "video" {
		kind = "track"
	}
	if link.Type == "album" {
		kind = "album"
	}
	return u.String(), kind, nil
}
