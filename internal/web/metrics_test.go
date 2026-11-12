// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOwnerMetricsTotalsPermissionsAndFailure(t *testing.T) {
	a, _, owner := ownerBrowser(t)
	metrics, err := a.store.Metrics(t.Context(), time.Now())
	if err != nil || len(metrics) < 5 || metrics[0].Value != 4 {
		t.Fatal(metrics, err)
	}
	member := login(t, a, "alice")
	guest := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	for _, path := range []string{"/admin/metrics", "/admin/metrics?owner=true&role=admin"} {
		w := owner.request("GET", path, nil)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), "Instance overview") {
			t.Fatal(w.Code, w.Body.String())
		}
		for _, client := range []*browser{member, guest} {
			w = client.request("GET", path, nil)
			if w.Code != 303 && w.Code != 403 {
				t.Fatal("metrics authorization", w.Code)
			}
			if strings.Contains(w.Body.String(), "Instance overview") {
				t.Fatal("metrics leaked")
			}
		}
	}
}
