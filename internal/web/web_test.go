// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/airencracken/comfylib/clientip"
	"github.com/airencracken/comfylib/token"
	"github.com/airencracken/songstead/internal/media"
	"github.com/airencracken/songstead/internal/store"
)

type browser struct {
	t       *testing.T
	a       *App
	cookies map[string]*http.Cookie
	csrf    string
}

func fixture(t *testing.T) (*App, []int64) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "songstead.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	var ids []int64
	for _, name := range []string{"alice", "bobby", "carol"} {
		id, err := s.CreateUser(context.Background(), name, "a-long-test-password")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	a, err := New(s, Config{})
	if err != nil {
		t.Fatal(err)
	}
	return a, ids
}

var csrfField = regexp.MustCompile(`name="csrf" value="([a-f0-9]{64})"`)

func (b *browser) request(method, path string, values url.Values) *httptest.ResponseRecorder {
	b.t.Helper()
	if values == nil {
		values = url.Values{}
	}
	if method == "POST" {
		values.Set("csrf", b.csrf)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
	r.RemoteAddr = "192.0.2.1:1234"
	if method == "POST" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, c := range b.cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	b.a.ServeHTTP(w, r)
	for _, c := range w.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(b.cookies, c.Name)
		} else {
			b.cookies[c.Name] = c
		}
	}
	if match := csrfField.FindStringSubmatch(w.Body.String()); len(match) > 0 {
		b.csrf = match[1]
	}
	// A login rotates the CSRF token before the redirect is followed.
	for name, c := range b.cookies {
		if strings.HasSuffix(name, "_csrf") {
			b.csrf = c.Value
		}
	}
	return w
}
func login(t *testing.T, a *App, name string) *browser {
	t.Helper()
	b := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	if w := b.request("GET", "/login", nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := b.request("POST", "/login", url.Values{"username": {name}, "password": {"a-long-test-password"}}); w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	return b
}

func TestTwoFriendsWithoutJavaScript(t *testing.T) {
	a, u := fixture(t)
	alice := login(t, a, "alice")
	bobby := login(t, a, "bobby")
	w := alice.request("POST", "/recommendations/new", url.Values{"url": {"https://example.org/album"}, "recipient": {strconv.FormatInt(u[1], 10)}, "note": {"listen on your next walk"}})
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	path := w.Header().Get("Location")
	w = bobby.request("GET", "/inbox", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "listen on your next walk") {
		t.Fatal("inbox missing recommendation", w.Code, w.Body.String())
	}
	if w := bobby.request("GET", path, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "selected>Unheard") {
		t.Fatal("opening changed state", w.Body.String())
	}
	w = bobby.request("POST", path+"/reaction", url.Values{"listening": {"listened"}, "rating": {"1"}, "personal_note": {"private-listening-note"}})
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = bobby.request("POST", path+"/comments", url.Values{"body": {"That bass line stayed with me."}})
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = alice.request("GET", path, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "That bass line stayed with me.") || strings.Contains(w.Body.String(), "private-listening-note") {
		t.Fatal("comment or reaction privacy failed", w.Body.String())
	}
	w = bobby.request("GET", "/history?status=listened", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), path) {
		t.Fatal("history filtering failed")
	}
	w = bobby.request("GET", "/account/export", nil)
	var archive store.Archive
	if err := json.Unmarshal(w.Body.Bytes(), &archive); err != nil || w.Code != 200 || archive.Format != "songstead-account" || len(archive.Recommendations) != 1 {
		t.Fatal("export contract failed", err, w.Body.String())
	}
	if w := bobby.request("POST", "/logout", nil); w.Code != 303 {
		t.Fatal(w.Code)
	}
	if w := bobby.request("GET", path, nil); w.Code != 303 {
		t.Fatal("logout did not revoke access")
	}
}

func TestRouteAndHTTPContracts(t *testing.T) {
	a, u := fixture(t)
	id, err := a.store.Recommend(context.Background(), u[0], u[1], "https://example.org/song", "")
	if err != nil {
		t.Fatal(err)
	}
	b := login(t, a, "bobby")
	for _, path := range []string{"/", "/shelf", "/inbox", "/recent", "/history", "/recommendations/new", "/recommendations/" + strconv.FormatInt(id, 10), "/login"} {
		w := b.request("GET", path, nil)
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
			t.Fatal("route failed", path, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "form-action 'self'") {
			t.Fatal("missing security headers")
		}
	}
	for _, path := range []string{"/static/app.css", "/static/theme.js", "/static/htmx.min.js", "/static/jukebox.png", "/healthz"} {
		if w := b.request("GET", path, nil); w.Code != 200 {
			t.Fatal("asset route missing", path, w.Code)
		}
	}
	for _, path := range []string{"/missing", "/recommendations/0", "/recommendations/nope", "/recommendations/999999", "/recommendations/9223372036854775808"} {
		if w := b.request("GET", path, nil); w.Code != 404 {
			t.Fatal("missing route did not return 404", path, w.Code)
		}
	}
	if w := b.request("POST", "/inbox", nil); w.Code != 405 {
		t.Fatal("read route accepted write", w.Code)
	}
	if w := b.request("GET", "/inbox?offset=-1", nil); w.Code != 400 {
		t.Fatal("negative offset accepted")
	}
	if w := b.request("GET", "/history?status=evil", nil); w.Code != 422 {
		t.Fatal("invalid filter accepted")
	}
	if w := b.request("HEAD", "/healthz", nil); w.Code != 200 {
		t.Fatal("health HEAD failed")
	}
}

func TestPrivateRoutesAndWrites(t *testing.T) {
	a, u := fixture(t)
	id, err := a.store.Recommend(context.Background(), u[0], u[1], "https://example.org/private", "secret-recommendation")
	if err != nil {
		t.Fatal(err)
	}
	carol := login(t, a, "carol")
	path := "/recommendations/" + strconv.FormatInt(id, 10)
	for _, p := range []string{path, "/inbox", "/history", "/account/export"} {
		w := carol.request("GET", p, nil)
		if strings.Contains(w.Body.String(), "secret-recommendation") {
			t.Fatal("private text exposed", p)
		}
		if p == path && w.Code != 404 {
			t.Fatal("private ID existence disclosed", w.Code)
		}
	}
	for suffix, values := range map[string]url.Values{"/reaction": {"listening": {"listened"}, "rating": {"1"}}, "/comments": {"body": {"intrusion"}}} {
		if w := carol.request("POST", path+suffix, values); w.Code != 404 {
			t.Fatal("private write allowed", w.Code, w.Body.String())
		}
	}
}

func TestCSRFSessionBindingAndOrigin(t *testing.T) {
	a, _ := fixture(t)
	b := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	b.request("GET", "/login", nil)
	anonymous := b.csrf
	b.request("POST", "/login", url.Values{"username": {"alice"}, "password": {"a-long-test-password"}})
	for _, candidate := range []string{"", anonymous, strings.Repeat("a", 64)} {
		r := httptest.NewRequest("POST", "/logout", strings.NewReader(url.Values{"csrf": {candidate}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, c := range b.cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("unbound CSRF accepted", w.Code)
		}
	}
	r := httptest.NewRequest("POST", "/logout", strings.NewReader(url.Values{"csrf": {b.csrf}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	for _, c := range b.cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-site form accepted")
	}
	// A planted CSRF cookie cannot replace a token derived from the session.
	b.cookies["songstead_csrf"] = &http.Cookie{Name: "songstead_csrf", Value: strings.Repeat("b", 64)}
	b.request("GET", "/inbox", nil)
	secret := b.cookies["songstead_session"].Value
	if b.csrf != token.SessionCSRF(secret, "songstead-csrf-v1") {
		t.Fatal("planted cookie became authority")
	}
}

func TestSecureCookiesAndProxyTrust(t *testing.T) {
	a, _ := fixture(t)
	trusted, err := clientip.ParseTrusted("127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	a.config.TrustedProxies = trusted
	for _, tt := range []struct {
		peer   string
		secure bool
	}{{"127.0.0.1:1234", true}, {"192.0.2.1:1234", false}} {
		r := httptest.NewRequest("GET", "/login", nil)
		r.RemoteAddr = tt.peer
		r.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		for _, c := range w.Result().Cookies() {
			if c.Secure != tt.secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Domain != "" {
				t.Fatal("cookie protection failed", c)
			}
			if strings.HasPrefix(c.Name, "__Host-") != tt.secure {
				t.Fatal("wrong cookie namespace")
			}
		}
	}
}

func TestEscapingAndMetadataFailure(t *testing.T) {
	a, u := fixture(t)
	ctx := context.Background()
	id, err := a.store.Recommend(ctx, u[0], u[1], "https://youtu.be/dQw4w9WgXcQ", `<script>alert('note')</script>`)
	if err != nil {
		t.Fatal(err)
	}
	a.store.Metadata(ctx, func(context.Context, string) (media.Metadata, error) {
		return media.Metadata{Title: `<script>alert('metadata')</script>`, Artist: `<img src=x onerror=alert(1)>`}, nil
	})
	a.store.AddComment(ctx, u[0], id, `<script>alert('comment')</script>`)
	b := login(t, a, "bobby")
	w := b.request("GET", "/recommendations/"+strconv.FormatInt(id, 10), nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "<script>alert") || strings.Contains(w.Body.String(), "<img src=x") || !strings.Contains(w.Body.String(), "&lt;script&gt;") {
		t.Fatal("untrusted text escaped incorrectly", w.Body.String())
	}
}

func TestAdversarialFormsAndLoginLimit(t *testing.T) {
	a, u := fixture(t)
	b := login(t, a, "alice")
	for _, raw := range []string{"javascript:alert(1)", "file:///etc/passwd", "https://user:password@example.org/song"} {
		w := b.request("POST", "/recommendations/new", url.Values{"url": {raw}, "recipient": {strconv.FormatInt(u[1], 10)}})
		if w.Code != 422 {
			t.Fatal("bad URL accepted", raw, w.Code)
		}
	}
	for _, recipient := range []string{"0", strconv.FormatInt(u[0], 10), "99999", "1 OR 1=1"} {
		if w := b.request("POST", "/recommendations/new", url.Values{"url": {"https://example.org"}, "recipient": {recipient}}); w.Code != 422 {
			t.Fatal("bad recipient accepted", recipient, w.Code)
		}
	}
	for _, tt := range []struct {
		body string
		code int
	}{{"csrf=" + b.csrf + "&body=one&body=two", 400}, {"csrf=" + b.csrf + "&body=%zz", 400}, {"csrf=" + b.csrf + "&body=" + strings.Repeat("x", 17000), 413}} {
		r := httptest.NewRequest("POST", "/logout", strings.NewReader(tt.body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, c := range b.cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		if w.Code != tt.code {
			t.Fatal("bad form status", w.Code, tt.code)
		}
	}
	x := &browser{t: t, a: a, cookies: map[string]*http.Cookie{}}
	x.request("GET", "/login", nil)
	for i := 0; i < 20; i++ {
		x.request("POST", "/login", url.Values{"username": {"missing"}, "password": {"wrong"}})
	}
	w := x.request("POST", "/login", url.Values{"username": {"missing"}, "password": {"wrong"}})
	if w.Code != 429 || w.Header().Get("Retry-After") != "900" {
		t.Fatal("login flood unlimited", w.Code)
	}
}
