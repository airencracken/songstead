// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"fmt"
	"strings"
	"testing"
)

// Metrics describe operational health. Sign-ins and opens never store who
// signed in or which recommendation was read.
func TestOpensAndSignInsRecordNoMemberOrItemActivity(t *testing.T) {
	a, users, owner := ownerBrowser(t)
	alice := login(t, a, "alice")
	bob := login(t, a, "bobby")
	id, err := a.store.Recommend(t.Context(), users[0], users[1], "https://example.org/song", "private activity fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, client := range []*browser{alice, bob, owner} {
		client.request("GET", fmt.Sprintf("/recommendations/%d", id), nil)
	}
	var rows int
	if err := a.store.DB().QueryRow("SELECT count(*) FROM comfy_analytics_activity").Scan(&rows); err != nil || rows != 0 {
		t.Fatal("per-member activity stored", rows, err)
	}
	w := owner.request("GET", "/admin/metrics", nil)
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "Instance overview") || strings.Contains(body, "private activity fixture") {
		t.Fatal(w.Code, body)
	}
	for _, name := range []string{">alice<", ">bobby<", fmt.Sprintf("Recommendation %d", id)} {
		if strings.Contains(body, name) {
			t.Fatalf("dashboard ranks %s", name)
		}
	}
	if w := bob.request("GET", "/admin/metrics?role=owner", nil); w.Code != 403 || strings.Contains(w.Body.String(), "Instance overview") {
		t.Fatal("member read metrics", w.Code)
	}
}
