// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"net/http"
	"net/url"
	"strconv"
)

const recentCommentsPageSize = 25

func (a *App) recentComments(w http.ResponseWriter, r *http.Request) {
	var before int64
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		a.render(w, r, 400, page{Title: "Recent comments", Error: "Choose a valid comments page."})
		return
	}
	if values, ok := query["before"]; ok {
		if len(values) != 1 || values[0] == "" {
			a.render(w, r, 400, page{Title: "Recent comments", Error: "Choose a valid comments page."})
			return
		}
		var err error
		before, err = strconv.ParseInt(values[0], 10, 64)
		if err != nil || before <= 0 {
			a.render(w, r, 400, page{Title: "Recent comments", Error: "Choose a valid comments page."})
			return
		}
	}
	comments, err := a.store.RecentComments(r.Context(), state(r).User.ID, before, recentCommentsPageSize+1)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p := page{View: "recent-comments", Title: "Recent comments", Recent: true, RecentComments: comments, HasNext: len(comments) > recentCommentsPageSize, CommentsBefore: before}
	if p.HasNext {
		p.RecentComments = comments[:recentCommentsPageSize]
		p.CommentsNext = p.RecentComments[len(p.RecentComments)-1].ID
	}
	a.render(w, r, 200, p)
}
