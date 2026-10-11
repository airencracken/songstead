// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"context"
	"github.com/airencracken/comfylib/autobackup"
	"strings"
	"testing"
)

func TestBackupStatusIsOwnerOnly(t *testing.T) {
	a, _, owner := ownerBrowser(t)
	cfg, err := autobackup.Load("songstead", t.TempDir(), func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	a.config.Backups, err = autobackup.New(cfg, func(context.Context, string, int64) error { return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	w := owner.request("GET", "/admin/settings", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Automatic backups") || !strings.Contains(w.Body.String(), cfg.Directory) || !strings.Contains(w.Body.String(), "No automatic backup has succeeded") {
		t.Fatal(w.Code, w.Body.String())
	}
	member := login(t, a, "alice")
	if w := member.request("GET", "/admin/settings", nil); w.Code != 403 || strings.Contains(w.Body.String(), cfg.Directory) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, path := range []string{"/healthz", "/about", "/members/1"} {
		w := member.request("GET", path, nil)
		if strings.Contains(w.Body.String(), cfg.Directory) {
			t.Fatal("private backup path leaked", path)
		}
	}
}
