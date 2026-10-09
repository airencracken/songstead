// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/airencracken/songstead/internal/store"
)

func (a *App) thumbnail(w http.ResponseWriter, r *http.Request) {
	id, err := recommendationID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	data, err := a.store.Artwork(r.Context(), state(r).User.ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func (a *App) saveLabels(w http.ResponseWriter, r *http.Request) {
	id, err := recommendationID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	tags, err := store.ParseLabels(r.PostForm.Get("tags"), 40)
	if err == nil {
		err = a.store.SetLabels(r.Context(), state(r).User.ID, id, store.Labels{Genre: r.PostForm.Get("genre"), Tags: tags})
	}
	if errors.Is(err, store.ErrInvalid) {
		a.showDetail(w, r, id, 422, "Use a single-line genre of up to 80 characters and up to 20 comma-separated tags of 40 characters each.")
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/recommendations/"+strconv.FormatInt(id, 10)+"?saved=labels#music-labels", 303)
}

func (a *App) accountAnnotations(w http.ResponseWriter, r *http.Request) {
	err := a.store.SetAnnotationMode(r.Context(), state(r).User.ID, r.PostForm.Get("mode"))
	if errors.Is(err, store.ErrInvalid) {
		a.showAccount(w, r, 422, page{Error: "Choose a spoiler preference from the list."})
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/account?saved=annotations#spoiler-preferences", 303)
}

func (a *App) accountDiscovery(w http.ResponseWriter, r *http.Request) {
	var prefs store.DiscoveryPreferences
	for _, field := range []struct {
		name   string
		target *[]string
		max    int
	}{
		{"excluded_genres", &prefs.ExcludedGenres, 80}, {"preferred_genres", &prefs.PreferredGenres, 80}, {"excluded_tags", &prefs.ExcludedTags, 40}, {"preferred_tags", &prefs.PreferredTags, 40},
	} {
		values, err := store.ParseLabels(r.PostForm.Get(field.name), field.max)
		if err != nil {
			// Keep the draft, including the invalid field, for correction.
			prefs.ExcludedGenres = strings.Split(r.PostForm.Get("excluded_genres"), ",")
			prefs.PreferredGenres = strings.Split(r.PostForm.Get("preferred_genres"), ",")
			prefs.ExcludedTags = strings.Split(r.PostForm.Get("excluded_tags"), ",")
			prefs.PreferredTags = strings.Split(r.PostForm.Get("preferred_tags"), ",")
			a.showAccount(w, r, 422, page{Error: "Use up to 20 comma-separated names per field, with genres up to 80 characters and tags up to 40. Names must fit on one line.", DiscoveryDraft: &prefs})
			return
		}
		*field.target = values
	}
	if err := a.store.SetDiscoveryPreferences(r.Context(), state(r).User.ID, prefs); err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/account?saved=discovery#discovery-preferences", 303)
}

func (a *App) showAccount(w http.ResponseWriter, r *http.Request, status int, p page) {
	var err error
	p.AnnotationMode, err = a.store.AnnotationMode(r.Context(), state(r).User.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p.DiscoveryPreferences, err = a.store.DiscoveryPreferences(r.Context(), state(r).User.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if p.DiscoveryDraft != nil {
		p.DiscoveryPreferences = *p.DiscoveryDraft
	}
	p.View, p.Title = "account", "Your account"
	if status == http.StatusOK && r.Method == http.MethodGet {
		switch r.URL.Query().Get("saved") {
		case "annotations":
			p.AnnotationNotice = "Spoiler preference saved for your account."
		case "discovery":
			p.Notice = "Discovery preferences saved. They apply to Recent."
		}
	}
	a.render(w, r, status, p)
}
