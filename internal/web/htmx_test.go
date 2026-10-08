// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"testing"
)

func TestHTMXValidationAndPrivateHistoryContract(t *testing.T) {
	a, _ := fixture(t)
	b := login(t, a, "alice")
	w := b.request("GET", "/recommendations/new", nil)
	match := regexp.MustCompile(`name="htmx-config" content='([^']+)'`).FindStringSubmatch(w.Body.String())
	if len(match) != 2 {
		t.Fatal("missing HTMX configuration")
	}
	var cfg struct {
		AllowEval        bool `json:"allowEval"`
		HistoryCacheSize int  `json:"historyCacheSize"`
		ResponseHandling []struct {
			Code string `json:"code"`
			Swap bool   `json:"swap"`
		} `json:"responseHandling"`
	}
	if err := json.Unmarshal([]byte(html.UnescapeString(match[1])), &cfg); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, response := range cfg.ResponseHandling {
		if response.Code == "422" && response.Swap {
			found = true
		}
	}
	if !found || cfg.AllowEval || cfg.HistoryCacheSize != 0 || !strings.Contains(w.Body.String(), `hx-history="false"`) {
		t.Fatal("validation hidden or private history cached")
	}
}
