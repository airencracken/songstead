// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"bytes"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestFaviconLinksAndHTTPContracts(t *testing.T) {
	a, _ := fixture(t)
	anonymous := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	for _, b := range []*browser{anonymous, login(t, a, "alice")} {
		for _, path := range []string{"/login", "/shelf"} {
			page := b.request("GET", path, nil)
			if page.Code == http.StatusSeeOther && b == anonymous {
				continue
			}
			if page.Code != http.StatusOK {
				t.Fatal("page route", path, page.Code)
			}
			for _, size := range []int{16, 32} {
				link := fmt.Sprintf(`<link rel="icon" type="image/png" sizes="%dx%d" href="/static/favicon-%d.png">`, size, size, size)
				if !strings.Contains(page.Body.String(), link) {
					t.Fatal("favicon missing from page", path, link)
				}
			}
		}
	}
	for _, size := range []int{16, 32} {
		path := fmt.Sprintf("/static/favicon-%d.png", size)
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest("GET", path, nil)
			// Static assets must work even when a browser sends a stale session.
			request.AddCookie(&http.Cookie{Name: "songstead_session", Value: strings.Repeat("a", 64)})
			w := httptest.NewRecorder()
			a.ServeHTTP(w, request)
			if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("favicon HTTP contract", w.Code, w.Header())
			}
			if len(w.Result().Cookies()) != 0 {
				t.Fatal("favicon request created session cookies")
			}
			img, err := png.Decode(bytes.NewReader(w.Body.Bytes()))
			if err != nil {
				t.Fatal("invalid PNG", err)
			}
			if img.Bounds().Dx() != size || img.Bounds().Dy() != size {
				t.Fatal("wrong icon dimensions", img.Bounds())
			}
			_, _, _, alpha := img.At(0, 0).RGBA()
			if alpha != 0 {
				t.Fatal("favicon lost transparent background")
			}
			visible := false
			for y := 0; y < size; y++ {
				for x := 0; x < size; x++ {
					_, _, _, alpha := img.At(x, y).RGBA()
					visible = visible || alpha > 0
				}
			}
			if !visible {
				t.Fatal("favicon is empty")
			}
			head := anonymous.request("HEAD", path, nil)
			if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Type") != "image/png" || head.Header().Get("Content-Length") != strconv.Itoa(w.Body.Len()) {
				t.Fatal("favicon HEAD contract", head.Code, head.Header())
			}
			if write := anonymous.request("POST", path, nil); write.Code != http.StatusMethodNotAllowed {
				t.Fatal("favicon accepted a write", write.Code)
			}
		})
	}
	if w := anonymous.request("GET", "/static/favicon-64.png", nil); w.Code != http.StatusNotFound {
		t.Fatal("missing favicon did not return 404", w.Code)
	}
}
