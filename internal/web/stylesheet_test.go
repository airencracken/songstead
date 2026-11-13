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

// Songstead maps its theme to the shared --comfy-* tokens once and reads them
// directly. Defining the older names Comfylib still falls back to (--bg,
// --panel, --text and so on) would loop through the deprecated fallback.
func TestStylesheetMapsSharedTokensOnce(t *testing.T) {
	css := stylesheet(t)
	block := regexp.MustCompile(`(?s)\n:root\s*\{(.*?)\}`).FindStringSubmatch(css)
	if block == nil {
		t.Fatal("missing :root tokens")
	}
	for _, token := range []string{"--comfy-surface", "--comfy-raised", "--comfy-border", "--comfy-control-border", "--comfy-text", "--comfy-muted", "--comfy-accent", "--comfy-accent-text", "--comfy-danger", "--comfy-font", "--comfy-radius"} {
		if !strings.Contains(block[1], token+":") {
			t.Errorf("missing %s", token)
		}
	}
	for _, old := range []string{"--background", "--bg", "--bg-elev", "--panel", "--paper", "--border", "--line", "--text", "--ink", "--muted", "--accent", "--green", "--button-text", "--danger", "--radius", "--comfy-ink", "--comfy-panel", "--comfy-line"} {
		if regexp.MustCompile(`(?m)^\s*`+regexp.QuoteMeta(old)+`\s*:`).MatchString(css) || strings.Contains(css, "var("+old+")") {
			t.Errorf("stylesheet still uses %s", old)
		}
	}
	if strings.Contains(css, ".invitation-") {
		t.Error("shared invitation pages read the tokens; app overrides are redundant")
	}
}
