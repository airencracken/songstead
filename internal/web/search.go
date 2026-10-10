// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/airencracken/songstead/internal/store"
)

const searchPageSize = 25

func searchParameters(r *http.Request) (string, string, int64, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return "", "", 0, store.ErrInvalid
	}
	for _, key := range []string{"q", "kind", "before"} {
		if len(values[key]) > 1 {
			return "", "", 0, store.ErrInvalid
		}
	}
	query, err := store.SearchQuery(values.Get("q"))
	if err != nil {
		return "", "", 0, err
	}
	kind := values.Get("kind")
	if kind == "" {
		kind = "music"
	}
	if kind != "music" && kind != "comments" {
		return "", "", 0, store.ErrInvalid
	}
	var before int64
	if raw, ok := values["before"]; ok {
		before, err = strconv.ParseInt(raw[0], 10, 64)
		if err != nil || before <= 0 {
			return "", "", 0, store.ErrInvalid
		}
	}
	return query, kind, before, nil
}

func (a *App) search(w http.ResponseWriter, r *http.Request) {
	query, kind, before, err := searchParameters(r)
	if err != nil {
		a.render(w, r, 400, page{View: "search", Title: "Search", SearchKind: "music", Error: "Enter a search of up to 200 characters and choose a valid page."})
		return
	}
	p := page{View: "search", Title: "Search", Query: query, SearchKind: kind, CommentsBefore: before}
	if kind == "comments" {
		p.RecentComments, err = a.store.SearchComments(r.Context(), state(r).User.ID, query, before, searchPageSize+1)
		if len(p.RecentComments) > searchPageSize {
			p.HasNext = true
			p.RecentComments = p.RecentComments[:searchPageSize]
			p.CommentsNext = p.RecentComments[searchPageSize-1].ID
		}
	} else {
		p.Items, err = a.store.SearchMusic(r.Context(), state(r).User.ID, query, before, searchPageSize+1)
		if len(p.Items) > searchPageSize {
			p.HasNext = true
			p.Items = p.Items[:searchPageSize]
			p.CommentsNext = p.Items[searchPageSize-1].ID
		}
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.render(w, r, 200, p)
}
