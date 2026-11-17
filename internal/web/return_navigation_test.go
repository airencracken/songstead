package web

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestRecommendationReturnsToFilteredRecent(t *testing.T) {
	app, _ := fixture(t)
	member := login(t, app, "alice")
	share := member.request("POST", "/recommendations/new", url.Values{
		"audience": {"members"}, "kind": {"album"},
		"url": {"https://music.example/afternoon"}, "title": {"Afternoon music"},
	})
	if share.Code != http.StatusSeeOther {
		t.Fatalf("sharing returned %d", share.Code)
	}
	recommendation := share.Header().Get("Location")
	const recent = "/recent?kind=album"
	list := member.request("GET", recent, nil)
	titleLink := regexp.MustCompile(`href="(` + regexp.QuoteMeta(recommendation) + `(?:\?[^"]*)?)">Afternoon music</a>`)
	match := titleLink.FindStringSubmatch(list.Body.String())
	if list.Code != http.StatusOK || len(match) != 2 {
		t.Fatalf("recommendation unavailable in filtered Recent (status %d)", list.Code)
	}
	detail := member.request("GET", html.UnescapeString(match[1]), nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail returned %d", detail.Code)
	}
	if !strings.Contains(detail.Body.String(), `href="`+recent+`"`) {
		t.Fatal("detail opened from filtered Recent does not link back to the same view")
	}
	reaction := member.request("POST", recommendation+"/reaction", url.Values{
		"listening": {"saved"}, "rating": {"0"}, "return_to": {recent},
	})
	path := reaction.Header().Get("Location")
	if reaction.Code != http.StatusSeeOther || !strings.Contains(path, "return_to=%2Frecent%3Fkind%3Dalbum") {
		t.Fatalf("saving listening feedback lost the origin: %d %s", reaction.Code, path)
	}
	w := member.request("GET", path, nil)
	if !strings.Contains(w.Body.String(), `href="`+recent+`"`) {
		t.Fatal("the detail page lost its originating list after a save")
	}
}

func TestBrowseReturnUsesLocalViewRoutes(t *testing.T) {
	for _, input := range []string{
		"//example.org/recent", "https://example.org/recent",
		"/account", "/recommendations/1", "/recent%2fcomments", "/invalid%00",
	} {
		if path, label := browseReturn(input); path != "" || label != "" {
			t.Errorf("%q returned %q (%q)", input, path, label)
		}
	}
	for _, input := range []string{"/recent", "/history?offset=50", "/search?q=old+records", "/recent/comments?before=45"} {
		if path, label := browseReturn(input + "#position"); path != input || label == "" {
			t.Errorf("%q returned %q (%q)", input, path, label)
		}
	}
}
