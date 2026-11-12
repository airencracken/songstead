// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestActivityCountsAuthorizedOpensAndSuccessfulLogins(t *testing.T) {
	a, users, owner := ownerBrowser(t)
	alice := login(t, a, "alice")
	bob := login(t, a, "bobby")
	id, err := a.store.Recommend(t.Context(), users[0], users[1], "https://example.org/song", "private activity fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{fmt.Sprintf("/recommendations/%d", id), fmt.Sprintf("/recommendations/%d", id), fmt.Sprintf("/recommendations/%04d", id)} {
		if w := bob.request("GET", path, nil); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := alice.request("GET", fmt.Sprintf("/recommendations/%d", id), nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	owner.request("GET", fmt.Sprintf("/recommendations/%d", id), nil)
	bob.request("HEAD", fmt.Sprintf("/recommendations/%d", id), nil)
	bob.request("GET", "/recommendations/999999", nil)
	bob.request("GET", "/shelf", nil)
	result, err := a.store.Analytics(t.Context(), time.Now())
	if err != nil || result.Logins != 3 || result.Views != 2 || result.ActiveMembers != 3 {
		t.Fatal(result, err)
	}
	if w := owner.request("GET", "/admin/metrics", nil); w.Code != 200 || strings.Contains(w.Body.String(), "private activity fixture") || !strings.Contains(w.Body.String(), "Recent sign-ins by account") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := bob.request("GET", "/admin/metrics?role=owner", nil); w.Code != 403 || strings.Contains(w.Body.String(), "Recent sign-ins") {
		t.Fatal("member read analytics", w.Code)
	}
	if _, err := a.store.DB().Exec("ALTER TABLE comfy_analytics_activity RENAME TO broken_activity"); err != nil {
		t.Fatal(err)
	}
	if w := owner.request("GET", "/admin/metrics", nil); w.Code != 500 || strings.Contains(w.Body.String(), "Instance overview") {
		t.Fatal("partial dashboard", w.Code)
	}
}
