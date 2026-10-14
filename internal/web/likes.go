// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"net/http"
	"net/url"
	"strconv"
)

func (a *App) likeSong(w http.ResponseWriter, r *http.Request) {
	id, err := recommendationID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	liked := r.PostForm.Get("liked")
	if len(r.PostForm["liked"]) != 1 || liked != "0" && liked != "1" {
		a.showDetail(w, r, id, 422, "Choose Like song or Unlike song.")
		return
	}
	if err := a.store.SetLiked(r.Context(), state(r).User.ID, id, liked == "1"); err != nil {
		a.fail(w, r, err)
		return
	}
	target := "/recommendations/" + strconv.FormatInt(id, 10) + "?saved=like#song-like"
	// Only return to our browse pages, preserving their filters and page.
	if back, err := url.Parse(r.PostForm.Get("return_to")); err == nil && !back.IsAbs() && back.Host == "" && back.Opaque == "" {
		switch back.Path {
		case "/shelf", "/inbox", "/recent", "/history":
			back.Fragment = ""
			target = back.String()
		}
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
