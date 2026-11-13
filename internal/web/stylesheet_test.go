// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"io/fs"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func stylesheet(t *testing.T) string {
	t.Helper()
	data, err := fs.ReadFile(assets, "static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	return regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(string(data), "")
}

// Meta text stays readable: no font size below 12px, with rem at 16px.
func TestStylesheetKeepsTwelvePixelFloor(t *testing.T) {
	css := stylesheet(t)
	sizes := regexp.MustCompile(`(?:font-size:\s*|font:\s*(?:\d+\s+)?)(\d*\.?\d+)(px|rem|em)`).FindAllStringSubmatch(css, -1)
	if len(sizes) == 0 {
		t.Fatal("no font sizes found")
	}
	for _, size := range sizes {
		value, err := strconv.ParseFloat(size[1], 64)
		if err != nil {
			t.Fatal(err)
		}
		if size[2] != "px" {
			value *= 16
		}
		if value < 12 {
			t.Errorf("%s%s is below 12px", size[1], size[2])
		}
	}
	for _, token := range regexp.MustCompile(`--text-[a-z]+:\s*(\d*\.?\d+)rem`).FindAllStringSubmatch(css, -1) {
		if value, _ := strconv.ParseFloat(token[1], 64); value*16 < 12 {
			t.Errorf("text token %srem is below 12px", token[1])
		}
	}
}

// Each top-level selector is declared once, so later overrides cannot hide
// the rule a reader edits.
func TestStylesheetHasNoPatchedOverrides(t *testing.T) {
	css := stylesheet(t)
	if strings.Count(css, "@media") > 0 {
		css = css[:strings.Index(css, "@media")]
	}
	seen := map[string]bool{}
	for _, rule := range regexp.MustCompile(`(?m)^([^{}@\n][^{}\n]*?)\s*\{`).FindAllStringSubmatch(css, -1) {
		selector := strings.Join(strings.Fields(rule[1]), " ")
		if seen[selector] {
			t.Errorf("selector %q is declared twice", selector)
		}
		seen[selector] = true
	}
	for _, line := range strings.Split(css, "\n") {
		if strings.Count(line, "{") > 1 {
			t.Errorf("several rules share one line: %s", line)
		}
	}
}

// Comfylib's shared pages read --comfy-* tokens. Songstead defines them with a
// selector that outranks the defaults in components.css.
func TestStylesheetDefinesSharedTokens(t *testing.T) {
	css := stylesheet(t)
	block := regexp.MustCompile(`(?s):root\[data-comfy-app="songstead"\]\s*\{(.*?)\}`).FindStringSubmatch(css)
	if block == nil {
		t.Fatal("missing scoped Comfylib tokens")
	}
	for _, token := range []string{"--comfy-ink", "--comfy-panel", "--comfy-line", "--comfy-accent", "--comfy-radius"} {
		if !strings.Contains(block[1], token+":") {
			t.Errorf("missing %s", token)
		}
	}
	for _, selector := range []string{`[data-comfy-app="songstead"] .invitation-header h1`, `[data-comfy-app="songstead"] .invitation-page h2`} {
		if !strings.Contains(css, selector) {
			t.Errorf("missing invitation override %s", selector)
		}
	}
}
