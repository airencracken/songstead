// SPDX-License-Identifier: AGPL-3.0-or-later
package annotations

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Reference struct{ Seconds, Start, End int }

// A complete colon token is examined, so an invalid 1:99:05 never yields 99:05.
var token = regexp.MustCompile(`[0-9]+(?::[0-9]+)+`)

// Parse accepts m:ss and h:mm:ss at word boundaries. Duration zero is unknown.
// URL fragments and clock-like tokens embedded in words are left as prose.
func Parse(body string, duration int) []Reference {
	out := []Reference{}
	for _, span := range token.FindAllStringIndex(body, -1) {
		a, b := span[0], span[1]
		beginning := strings.LastIndexAny(body[:a], " \t\n\r") + 1
		current := strings.ToLower(strings.TrimLeft(body[beginning:a], "([{\"'"))
		if strings.HasPrefix(current, "http://") || strings.HasPrefix(current, "https://") {
			continue
		}
		if a > 0 {
			r, _ := utf8.DecodeLastRuneInString(body[:a])
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ':' || r == '/' || r == '=' || r == '#' {
				continue
			}
		}
		if b < len(body) {
			r, _ := utf8.DecodeRuneInString(body[b:])
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ':' {
				continue
			}
		}
		seconds, ok := Seconds(body[a:b])
		if !ok || duration > 0 && seconds > duration {
			continue
		}
		out = append(out, Reference{seconds, a, b})
	}
	return out
}

func Seconds(raw string) (int, bool) {
	parts := []string{}
	start := 0
	for i, c := range raw {
		if c == ':' {
			parts = append(parts, raw[start:i])
			start = i + 1
		}
	}
	parts = append(parts, raw[start:])
	if len(parts) != 2 && len(parts) != 3 {
		return 0, false
	}
	total := 0
	for i, p := range parts {
		if len(p) == 0 || len(p) > 5 || i > 0 && len(p) != 2 {
			return 0, false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return 0, false
			}
		}
		n, err := strconv.Atoi(p)
		if err != nil || i > 0 && n > 59 {
			return 0, false
		}
		total = total*60 + n
	}
	return total, total <= 86400
}
func Format(seconds int) string {
	if seconds >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}
