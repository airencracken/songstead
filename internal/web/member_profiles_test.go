// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/airencracken/comfylib/memberprofile"
)

func TestMemberProfileRoutesOwnershipPrivacyAndExport(t *testing.T) {
	a, ids := fixture(t)
	alice, bobby := login(t, a, "alice"), login(t, a, "bobby")
	path := fmt.Sprintf("/members/%d", ids[0])
	if w := bobby.request("GET", path, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "hasn’t added a bio") {
		t.Fatal("optional blank profile", w.Code, w.Body.String())
	}
	form := url.Values{"profile_name": {"<script>name</script>"}, "profile_bio": {"A short bio.\n<script>bio</script>"}, "profile_link_label": {"My site", "", "", "", ""}, "profile_link_url": {"https://example.org/?x=1&y=2", "", "", "", ""}, "user_id": {fmt.Sprint(ids[1])}}
	w := alice.request("POST", "/account/profile", form)
	if w.Code != 303 || w.Header().Get("Location") != "/account?saved=profile#your-profile" {
		t.Fatal("save contract", w.Code, w.Body.String())
	}
	before, err := a.store.MemberProfile(t.Context(), ids[0])
	if err != nil {
		t.Fatal(err)
	}
	other, err := a.store.MemberProfile(t.Context(), ids[1])
	if err != nil || other.Biography.Name != "" {
		t.Fatal("edit targeted another member", other, err)
	}
	w = bobby.request("GET", path, nil)
	body := w.Body.String()
	for _, text := range []string{"&lt;script&gt;name&lt;/script&gt;", "&lt;script&gt;bio&lt;/script&gt;", `href="https://example.org/?x=1&amp;y=2"`, `target="_blank"`, `rel="noopener noreferrer ugc nofollow"`, `hx-boost="false"`, "signed-in members"} {
		if !strings.Contains(body, text) {
			t.Fatal("profile missing safe content", text, body)
		}
	}
	if strings.Contains(body, "<script>bio") || strings.Contains(body, "Edit your profile") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("profile privacy or escaping", body)
	}
	for _, method := range []string{"GET", "HEAD"} {
		guest := httptest.NewRecorder()
		a.ServeHTTP(guest, httptest.NewRequest(method, path, nil))
		if guest.Code != 303 || strings.Contains(guest.Body.String(), "short bio") {
			t.Fatal("guest profile disclosure", method, guest.Code)
		}
	}
	for _, id := range []string{"0", "-1", "abc", "9223372036854775808", "99999"} {
		if w := bobby.request("GET", "/members/"+id, nil); w.Code != 404 {
			t.Fatal("unavailable profile", id, w.Code)
		}
	}
	if w := bobby.request("HEAD", path, nil); w.Code != 200 {
		t.Fatal("HEAD profile route", w.Code)
	}
	if w := bobby.request("POST", path, nil); w.Code != 405 {
		t.Fatal("profile method contract", w.Code)
	}
	for _, bad := range []url.Values{{"profile_name": {"one", "two"}}, {"profile_bio": {"one", "two"}}, {"profile_link_label": {"Site"}, "profile_link_url": {"javascript:bad"}}, {"profile_bio": {strings.Repeat("x", 1001)}}} {
		w = alice.request("POST", "/account/profile", bad)
		want := 422
		if len(bad["profile_name"]) > 1 || len(bad["profile_bio"]) > 1 {
			want = 400
		}
		if w.Code != want {
			t.Fatal("invalid profile accepted", bad, w.Code)
		}
		after, err := a.store.MemberProfile(t.Context(), ids[0])
		if err != nil || !reflect.DeepEqual(after, before) {
			t.Fatal("invalid edit changed stored profile", after, err)
		}
	}
	csrf := alice.csrf
	alice.csrf = "wrong"
	if w := alice.request("POST", "/account/profile", url.Values{"profile_name": {"Hacked"}}); w.Code != 403 {
		t.Fatal("CSRF bypass", w.Code)
	}
	alice.csrf = csrf
	w = alice.request("GET", "/account?saved=profile", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Profile saved.") || !strings.Contains(w.Body.String(), `href="`+path+`"`) || strings.Count(w.Body.String(), `name="profile_link_url"`) != memberprofile.MaxLinks {
		t.Fatal("profile settings discoverability", w.Code, w.Body.String())
	}
	w = alice.request("GET", "/account/export", nil)
	var archive struct {
		Version   int
		Biography memberprofile.Profile `json:"profile_details"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &archive) != nil || archive.Version != 5 || !reflect.DeepEqual(archive.Biography, before.Biography) {
		t.Fatal("own profile export", w.Code, w.Body.String())
	}
	if w := bobby.request("GET", "/account/export", nil); strings.Contains(w.Body.String(), "short bio") {
		t.Fatal("another member's export contains profile")
	}
	if err := a.store.SetMemberProfile(t.Context(), ids[0], memberprofile.Profile{}); err != nil {
		t.Fatal(err)
	}
	if w := bobby.request("GET", path, nil); !strings.Contains(w.Body.String(), "hasn’t added a bio") {
		t.Fatal("cleared profile still visible")
	}
}

func TestMemberProfileFormArraysAreRouteScoped(t *testing.T) {
	a, _ := fixture(t)
	b := login(t, a, "alice")
	for _, test := range []struct {
		path   string
		form   url.Values
		status int
	}{
		{"/account/annotations", url.Values{"mode": {"immediate"}, "profile_link_label": {"one", "two"}, "profile_link_url": {"https://example.org/", "https://example.org/"}}, 400},
		{"/account/profile", url.Values{"profile_link_label": make([]string, 6), "profile_link_url": make([]string, 6)}, 400},
		{"/account/profile", url.Values{"profile_link_label": {"label", "other"}, "profile_link_url": {"https://example.org/"}}, 422},
	} {
		if w := b.request("POST", test.path, test.form); w.Code != test.status {
			t.Fatal("profile field exception escaped its route or bounds", test.path, w.Code)
		}
	}
}
