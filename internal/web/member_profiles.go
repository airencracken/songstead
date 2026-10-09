// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/airencracken/comfylib/memberprofile"
	"github.com/airencracken/songstead/internal/store"
)

func (a *App) memberProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	p, err := a.store.MemberProfile(r.Context(), id)
	if errors.Is(err, store.ErrMissing) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.render(w, r, 200, page{View: "member-profile", Title: p.Username + "'s profile", ProfileMember: p})
}

func (a *App) saveMemberProfile(w http.ResponseWriter, r *http.Request) {
	p, err := memberprofile.ParseForm(r.PostForm)
	if err != nil {
		a.showAccount(w, r, 422, page{Error: err.Error(), BiographyDraft: &p})
		return
	}
	if err = a.store.SetMemberProfile(r.Context(), state(r).User.ID, p); err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/account?saved=profile#your-profile", 303)
}
