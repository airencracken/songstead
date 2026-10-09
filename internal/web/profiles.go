// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/airencracken/comfylib/brandimage"
	"github.com/airencracken/songstead/internal/store"
)

func (a *App) profilePicture(w http.ResponseWriter, r *http.Request) {
	id, err := recommendationID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	data, kind, err := a.store.Picture(r.Context(), state(r).User.ID, id, r.URL.Query().Get("still") == "1")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if len(data) == 0 {
		data, err = assets.ReadFile("static/favicon-32.png")
		if err != nil {
			a.fail(w, r, err)
			return
		}
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func (a *App) saveProfilePicture(w http.ResponseWriter, r *http.Request) {
	var data []byte
	if r.PostForm.Get("remove_picture") != "1" {
		file, _, err := r.FormFile("picture")
		if err != nil {
			a.showAccount(w, r, 422, page{Error: "Choose a PNG, JPEG or GIF profile picture."})
			return
		}
		defer file.Close()
		data, err = io.ReadAll(io.LimitReader(file, brandimage.MaxBytes+1))
		if err != nil {
			a.fail(w, r, err)
			return
		}
	} else if r.MultipartForm != nil && len(r.MultipartForm.File) > 0 {
		a.showAccount(w, r, 422, page{Error: "Choose either a replacement picture or removal."})
		return
	}
	err := a.store.SetPicture(r.Context(), state(r).User.ID, data)
	if errors.Is(err, store.ErrInvalid) {
		a.showAccount(w, r, 422, page{Error: "Choose a PNG, JPEG or GIF up to 2 MiB and 512 by 512 pixels. GIFs may have up to 64 frames."})
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/account?saved=picture#profile-picture", 303)
}

func (a *App) saveAnimationPreference(w http.ResponseWriter, r *http.Request) {
	value := r.PostForm.Get("animate")
	if value != "" && value != "1" {
		a.showAccount(w, r, 422, page{Error: "Choose a valid animation preference."})
		return
	}
	if err := a.store.SetAnimationPreference(r.Context(), state(r).User.ID, value == "1"); err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/account?saved=animation#profile-picture", 303)
}
