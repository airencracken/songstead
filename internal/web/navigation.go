// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"net/http"
	"net/url"
	"strconv"
)

func browseReturn(raw string) (path, label string) {
	back, err := url.Parse(raw)
	if err != nil || back.IsAbs() || back.Host != "" || back.Opaque != "" || back.RawPath != "" {
		return "", ""
	}
	switch back.Path {
	case "/shelf", "/inbox":
		label = "your shelf"
		back.Path = "/shelf"
	case "/recent":
		label = "Recent"
	case "/recent/comments":
		label = "recent comments"
	case "/history":
		label = "your history"
	case "/search":
		label = "search"
	default:
		return "", ""
	}
	back.Fragment = ""
	return back.RequestURI(), label
}

func recommendationLink(id int64, back string, fragments ...string) string {
	target := &url.URL{Path: "/recommendations/" + strconv.FormatInt(id, 10)}
	if len(fragments) != 0 {
		target.Fragment = fragments[0]
	}
	if path, _ := browseReturn(back); path != "" {
		target.RawQuery = url.Values{"return_to": {path}}.Encode()
	}
	return target.String()
}

func detailReturn(r *http.Request) (string, string) {
	if back := r.FormValue("detail_return_to"); back != "" {
		return browseReturn(back)
	}
	return browseReturn(r.FormValue("return_to"))
}

func redirectDetail(w http.ResponseWriter, r *http.Request, id int64, notice, fragment string) {
	back, _ := detailReturn(r)
	target, _ := url.Parse(recommendationLink(id, back, fragment))
	if notice != "" {
		query := target.Query()
		query.Set("saved", notice)
		target.RawQuery = query.Encode()
	}
	http.Redirect(w, r, target.String(), http.StatusSeeOther)
}
