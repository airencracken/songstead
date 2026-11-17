// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"net/http"
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
	// Likes submitted from a list go directly back to its saved view.
	if back, _ := browseReturn(r.PostForm.Get("return_to")); back != "" {
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	redirectDetail(w, r, id, "like", "song-like")
}
