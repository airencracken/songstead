// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"bytes"
	"github.com/airencracken/songstead/internal/store"
	"html"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func ownerBrowser(t *testing.T) (*App, []int64, *browser) {
	t.Helper()
	a, u := fixture(t)
	if _, err := a.store.CreateOwner(t.Context(), "owner", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	return a, u, login(t, a, "owner")
}
func settingsValues() url.Values {
	return url.Values{"name": {"Friends"}, "welcome_title": {"Come listen"}, "welcome_text": {"Music for friends"}, "house_rules": {"Be kind"}, "owner_contact": {"Contact your host"}, "source_url": {"https://github.com/airencracken/songstead"}, "base_url": {"https://music.example.org"}, "witmoot_url": {"https://boards.example.org/forum"}, "join_mode": {"invite"}, "show_version": {"1"}}
}
func secretFromPage(t *testing.T, body, kind string) string {
	t.Helper()
	pattern := regexp.MustCompile(`value="([^"]*/` + kind + `\?[^"]+)"`)
	m := pattern.FindStringSubmatch(body)
	if len(m) != 2 {
		t.Fatal("missing private link", body)
	}
	return html.UnescapeString(m[1])
}
func TestOwnerAdministrationEndToEnd(t *testing.T) {
	a, u, owner := ownerBrowser(t)
	a.config.Version = "0.2.0-test"
	for _, path := range []string{"/admin", "/admin/settings", "/admin/users", "/admin/invites", "/invites", "/account", "/about"} {
		w := owner.request("GET", path, nil)
		if w.Code != 200 {
			t.Fatal("missing route", path, w.Code, w.Body.String())
		}
	}
	if w := owner.request("GET", "/shelf", nil); !strings.Contains(w.Body.String(), `href="/admin/settings"`) {
		t.Fatal("owner has no Admin link")
	}
	if w := owner.request("POST", "/admin/settings", settingsValues()); w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := owner.request("GET", "/admin/settings?saved=1", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "0.2.0-test") || !strings.Contains(w.Body.String(), "Changes are already in effect") {
		t.Fatal(w.Code, w.Body.String())
	}
	guest := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	if w := guest.request("GET", "/login", nil); !strings.Contains(w.Body.String(), "Come listen") || !strings.Contains(w.Body.String(), "Friends") {
		t.Fatal("saved identity not applied", w.Body.String())
	}
	if w := guest.request("GET", "/about", nil); !strings.Contains(w.Body.String(), "Be kind") || !strings.Contains(w.Body.String(), "Contact your host") {
		t.Fatal("public rules/contact missing")
	}
	w = owner.request("POST", "/invites", url.Values{"label": {"For a guest"}, "max_uses": {"1"}, "days": {"7"}})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	link := secretFromPage(t, w.Body.String(), "join")
	parsed, err := url.Parse(link)
	if err != nil || parsed.Host != "music.example.org" {
		t.Fatal("wrong invite origin", err)
	}
	raw := parsed.Query().Get("invite")
	if w := owner.request("GET", "/invites", nil); strings.Contains(w.Body.String(), raw) {
		t.Fatal("invite secret recoverable in listing")
	}
	for i := 0; i < 2; i++ {
		if w := guest.request("GET", parsed.RequestURI(), nil); w.Code != 200 {
			t.Fatal("GET consumed invitation", w.Code)
		}
	}
	if w := guest.request("POST", "/join", url.Values{"invite": {raw}, "username": {"guest"}, "password": {"a-long-test-password"}, "confirmation": {"a-long-test-password"}}); w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := guest.request("GET", parsed.RequestURI(), nil); w.Code != 422 {
		t.Fatal("used invite accepted", w.Code)
	}
	signedGuest := login(t, a, "guest")
	if w := signedGuest.request("GET", "/shelf", nil); w.Code != 200 || strings.Contains(w.Body.String(), `href="/admin/settings"`) {
		t.Fatal("guest privileges", w.Code)
	}
	recipient, _, err := a.store.Credentials(t.Context(), "guest")
	if err != nil {
		t.Fatal(err)
	}
	alice := login(t, a, "alice")
	w = alice.request("POST", "/recommendations/new", url.Values{"url": {"https://music.example.org/album"}, "recipient": {strconv.FormatInt(recipient.ID, 10)}, "note": {"Private provenance"}})
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	path := w.Header().Get("Location")
	if w := owner.request("GET", path, nil); w.Code != 404 {
		t.Fatal("owner read private recommendation", w.Code)
	}
	if w := signedGuest.request("POST", path+"/reaction", url.Values{"listening": {"listened"}, "rating": {"1"}, "personal_note": {"never-send-this-private-note"}}); w.Code != 303 {
		t.Fatal(w.Code)
	}
	w = signedGuest.request("POST", path+"/share", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "https://boards.example.org/forum") || strings.Contains(w.Body.String(), "never-send-this-private-note") || strings.Contains(w.Body.String(), "Private provenance") {
		t.Fatal("configured handoff/privacy failed", w.Code, w.Body.String())
	}
	restarted, err := New(a.store, a.config)
	if err != nil {
		t.Fatal(err)
	}
	signedGuest.a = restarted
	if w := signedGuest.request("POST", path+"/share", nil); w.Code != 200 || !strings.Contains(w.Body.String(), "https://boards.example.org/forum") {
		t.Fatal("settings lost on restart", w.Code, w.Body.String())
	}
	_ = u
}
func TestAdministrationPermissionAndCSRFContracts(t *testing.T) {
	a, u, owner := ownerBrowser(t)
	member := login(t, a, "alice")
	anon := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	anon.request("GET", "/login", nil)
	for _, path := range []string{"/admin", "/admin/settings", "/admin/users", "/admin/invites", "/invites"} {
		if w := member.request("GET", path, nil); w.Code != 403 {
			t.Fatal("member accessed owner route", path, w.Code)
		}
		if w := anon.request("GET", path, nil); w.Code != 303 {
			t.Fatal("anonymous admin access", path, w.Code)
		}
	}
	target := strconv.FormatInt(u[1], 10)
	for _, path := range []string{"/admin/settings", "/admin/settings/reset", "/admin/settings/images", "/admin/users/" + target, "/admin/users/" + target + "/reset", "/admin/users/" + target + "/reset/cancel", "/admin/invites", "/invites"} {
		if w := member.request("POST", path, settingsValues()); w.Code != 403 {
			t.Fatal("member mutation allowed", path, w.Code)
		}
	}
	for _, path := range []string{"/admin/settings", "/admin/settings/reset", "/admin/users/" + target, "/admin/users/" + target + "/reset", "/invites", "/join", "/reset", "/account/password"} {
		r := httptest.NewRequest("POST", path, strings.NewReader("csrf=wrong"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, c := range owner.cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("missing CSRF enforcement", path, w.Code)
		}
	}
	if err := a.store.ChangeAccount(t.Context(), stateUserID(t, a, "owner"), u[0], "can_invite", "1"); err != nil {
		t.Fatal(err)
	}
	if w := member.request("GET", "/invites", nil); w.Code != 200 {
		t.Fatal("delegate cannot invite", w.Code)
	}
	if w := member.request("GET", "/admin/invites", nil); w.Code != 403 {
		t.Fatal("delegate accessed all invitations")
	}
	if w := member.request("POST", "/invites", url.Values{"label": {"Guest"}, "days": {"7"}, "max_uses": {"1"}}); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}
func stateUserID(t *testing.T, a *App, name string) int64 {
	t.Helper()
	u, _, err := a.store.Credentials(t.Context(), name)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}
func TestRecoveryAndSuspensionThroughBrowser(t *testing.T) {
	a, u, owner := ownerBrowser(t)
	alice := login(t, a, "alice")
	target := strconv.FormatInt(u[0], 10)
	w := owner.request("POST", "/admin/users/"+target+"/reset", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	link := secretFromPage(t, w.Body.String(), "reset")
	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	raw := parsed.Query().Get("token")
	guest := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	for i := 0; i < 2; i++ {
		if w := guest.request("GET", parsed.RequestURI(), nil); w.Code != 200 {
			t.Fatal("GET consumed recovery", w.Code)
		}
	}
	if w := guest.request("POST", "/reset", url.Values{"token": {raw}, "password": {"a-new-test-password"}, "confirmation": {"does-not-match"}}); w.Code != 422 {
		t.Fatal("confirmation ignored", w.Code)
	}
	if w := guest.request("POST", "/reset", url.Values{"token": {raw}, "password": {"a-new-test-password"}, "confirmation": {"a-new-test-password"}}); w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := alice.request("GET", "/shelf", nil); w.Code != 303 {
		t.Fatal("recovery retained session", w.Code)
	}
	if w := guest.request("GET", parsed.RequestURI(), nil); w.Code != 422 {
		t.Fatal("recovery replay accepted", w.Code)
	}
	guest.request("GET", "/login", nil)
	if w := guest.request("POST", "/login", url.Values{"username": {"alice"}, "password": {"a-new-test-password"}}); w.Code != 303 {
		t.Fatal("new password cannot log in", w.Code)
	}
	if w := owner.request("POST", "/admin/users/"+target, url.Values{"action": {"suspended"}, "value": {"1"}}); w.Code != 303 {
		t.Fatal(w.Code)
	}
	if w := guest.request("GET", "/shelf", nil); w.Code != 303 {
		t.Fatal("suspended account session works", w.Code)
	}
	guest.request("GET", "/login", nil)
	if w := guest.request("POST", "/login", url.Values{"username": {"alice"}, "password": {"a-new-test-password"}}); w.Code != 422 {
		t.Fatal("suspended account signed in", w.Code)
	}
	if w := owner.request("POST", "/admin/users/"+strconv.FormatInt(stateUserID(t, a, "owner"), 10), url.Values{"action": {"role"}, "value": {"member"}}); w.Code != 422 {
		t.Fatal("last owner removed", w.Code)
	}
}
func TestSettingsInvalidDraftEscapingAndBlankIntegration(t *testing.T) {
	a, _, owner := ownerBrowser(t)
	values := settingsValues()
	values.Set("name", "<script>alert(1)</script>")
	values.Set("witmoot_url", "javascript:alert(1)")
	w := owner.request("POST", "/admin/settings", values)
	if w.Code != 422 || strings.Contains(w.Body.String(), "<script>alert(1)") || !strings.Contains(w.Body.String(), "&lt;script&gt;") {
		t.Fatal("invalid draft lost or unescaped", w.Code, w.Body.String())
	}
	values.Set("witmoot_url", "")
	if w := owner.request("POST", "/admin/settings", values); w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	a.config.WitmootURL = "https://env.example.org"
	settings, err := a.store.Settings(t.Context(), store.DefaultSettings(a.config.BaseURL, a.config.WitmootURL))
	if err != nil || settings.WitmootURL != "" {
		t.Fatal("blank does not disable env integration", settings, err)
	}
	if w := owner.request("POST", "/admin/settings/reset", nil); w.Code != 303 {
		t.Fatal(w.Code)
	}
	settings, err = a.store.Settings(t.Context(), store.DefaultSettings(a.config.BaseURL, a.config.WitmootURL))
	if err != nil || settings.WitmootURL != "https://env.example.org" {
		t.Fatal("defaults restoration ignored env", settings, err)
	}
}
func imageRequest(t *testing.T, b *browser, files map[string][][]byte, fields url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("csrf", b.csrf); err != nil {
		t.Fatal(err)
	}
	for key, values := range fields {
		for _, v := range values {
			if err := writer.WriteField(key, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	for key, values := range files {
		for _, v := range values {
			part, err := writer.CreateFormFile(key, "image.png")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write(v); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/admin/settings/images", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	for _, c := range b.cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	b.a.ServeHTTP(w, r)
	return w
}
func TestBrandingMultipartRoutesAndAtomicity(t *testing.T) {
	a, _, owner := ownerBrowser(t)
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewGray(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	img := out.Bytes()
	if w := imageRequest(t, owner, map[string][][]byte{"mascot": {img}, "favicon": {[]byte("bad")}}, nil); w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := owner.request("GET", "/branding/mascot", nil); w.Code != 404 {
		t.Fatal("partial image save", w.Code)
	}
	if w := imageRequest(t, owner, map[string][][]byte{"mascot": {img, img}}, nil); w.Code != 400 {
		t.Fatal("duplicate upload accepted", w.Code)
	}
	if w := imageRequest(t, owner, map[string][][]byte{"mascot": {img}}, url.Values{"remove_mascot": {"0", "1"}}); w.Code != 400 {
		t.Fatal("duplicate multipart field accepted", w.Code)
	}
	if w := imageRequest(t, owner, map[string][][]byte{"mascot": {img}, "favicon": {img}}, nil); w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, name := range []string{"mascot", "favicon"} {
		w := owner.request("GET", "/branding/"+name, nil)
		if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
			t.Fatal("branding asset contract", w.Code)
		}
		if _, err := png.Decode(bytes.NewReader(w.Body.Bytes())); err != nil {
			t.Fatal(err)
		}
		if w := owner.request("HEAD", "/branding/"+name, nil); w.Code != 200 || w.Body.Len() != 0 {
			t.Fatal("HEAD asset contract", w.Code)
		}
	}
	if w := owner.request("GET", "/login", nil); !strings.Contains(w.Body.String(), `href="/branding/favicon"`) || !strings.Contains(w.Body.String(), `src="/branding/mascot"`) {
		t.Fatal("saved assets not used")
	}
	if w := imageRequest(t, owner, nil, url.Values{"remove_favicon": {"1"}, "remove_mascot": {"1"}}); w.Code != 303 {
		t.Fatal(w.Code)
	}
	if w := owner.request("GET", "/branding/favicon", nil); w.Code != 404 {
		t.Fatal("removed asset still served")
	}
	if w := owner.request("GET", "/branding/script", nil); w.Code != 404 {
		t.Fatal("unknown asset route accepted")
	}
	if w := imageRequest(t, owner, map[string][][]byte{"favicon": {make([]byte, (5<<20)+1)}}, nil); w.Code != 413 {
		t.Fatal("unbounded multipart upload", w.Code)
	}
	_ = a
}

func TestAccountPasswordAndAdminMethodContracts(t *testing.T) {
	a, _, owner := ownerBrowser(t)
	member := login(t, a, "alice")
	for _, values := range []url.Values{
		{"current_password": {"wrong"}, "password": {"a-new-test-password"}, "confirmation": {"a-new-test-password"}},
		{"current_password": {"a-long-test-password"}, "password": {"a-new-test-password"}, "confirmation": {"mismatched"}},
		{"current_password": {"a-long-test-password"}, "password": {"short"}, "confirmation": {"short"}},
	} {
		if w := member.request("POST", "/account/password", values); w.Code != 422 {
			t.Fatal("invalid change accepted", w.Code)
		}
	}
	if w := member.request("GET", "/shelf", nil); w.Code != 200 {
		t.Fatal("failed password change ended session", w.Code)
	}
	if w := member.request("POST", "/account/password", url.Values{"current_password": {"a-long-test-password"}, "password": {"a-new-test-password"}, "confirmation": {"a-new-test-password"}}); w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := member.request("GET", "/shelf", nil); w.Code != 303 {
		t.Fatal("password change retained session", w.Code)
	}
	member.request("GET", "/login", nil)
	if w := member.request("POST", "/login", url.Values{"username": {"alice"}, "password": {"a-new-test-password"}}); w.Code != 303 {
		t.Fatal("new password rejected", w.Code)
	}
	for _, path := range []string{"/admin/settings/reset", "/admin/settings/images", "/admin/users/1/reset", "/admin/users/1/reset/cancel", "/invites/1/revoke", "/account/password"} {
		if w := owner.request("GET", path, nil); w.Code != 405 {
			t.Fatal("mutation route accepts GET", path, w.Code)
		}
	}
	for _, path := range []string{"/admin/settings", "/admin/users", "/invites", "/join", "/reset", "/about"} {
		if w := owner.request("PUT", path, nil); w.Code != 403 {
			t.Fatal("unsafe method skipped CSRF", path, w.Code)
		}
	}
	_ = a
}
