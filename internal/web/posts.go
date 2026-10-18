// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/airencracken/songstead/internal/store"
)

func (a *App) ownedRecommendation(w http.ResponseWriter, r *http.Request) (store.Recommendation, bool) {
	id, err := recommendationID(r)
	var item store.Recommendation
	if err == nil {
		item, err = a.store.Recommendation(r.Context(), state(r).User.ID, id)
	}
	if err == nil && item.SenderID != state(r).User.ID {
		err = store.ErrMissing
	}
	if err != nil {
		a.fail(w, r, err)
		return item, false
	}
	return item, true
}

func (a *App) editRecommendation(w http.ResponseWriter, r *http.Request) {
	item, ok := a.ownedRecommendation(w, r)
	if !ok {
		return
	}
	p := page{View: "compose", Title: "Edit your post", Item: item, URL: item.URL, MusicTitle: item.Title, Artist: item.Artist, Kind: item.Kind, Note: item.Note, Genre: item.Genre, Tags: strings.Join(item.Tags, ", ")}
	if r.Method == http.MethodGet {
		a.render(w, r, 200, p)
		return
	}
	p.URL, p.MusicTitle, p.Artist, p.Kind, p.Note, p.Genre, p.Tags = r.PostForm.Get("url"), r.PostForm.Get("title"), r.PostForm.Get("artist"), r.PostForm.Get("kind"), r.PostForm.Get("note"), r.PostForm.Get("genre"), r.PostForm.Get("tags")
	tags, err := store.ParseLabels(p.Tags, 40)
	if err == nil {
		err = a.store.EditPost(r.Context(), state(r).User.ID, item.ID, store.PostEdit{URL: p.URL, Title: p.MusicTitle, Artist: p.Artist, Kind: p.Kind, Note: p.Note, Labels: store.Labels{Genre: p.Genre, Tags: tags}})
	}
	if errors.Is(err, store.ErrInvalid) {
		p.Error = "Use a complete music URL, a title and artist of up to 160 characters, a note of up to 2,000, a genre of up to 80, and up to 20 tags of 40 characters each. A post with timestamped comments must keep the same music link."
		a.render(w, r, 422, p)
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/recommendations/"+strconv.FormatInt(item.ID, 10)+"?saved=post", 303)
}

func (a *App) deleteRecommendation(w http.ResponseWriter, r *http.Request) {
	item, ok := a.ownedRecommendation(w, r)
	if !ok {
		return
	}
	p := page{View: "delete-post", Title: "Delete your post", Item: item}
	if r.Method == http.MethodGet {
		a.render(w, r, 200, p)
		return
	}
	if r.PostForm.Get("confirm") != "delete" {
		p.Error = "Confirm that you want to delete this post."
		a.render(w, r, 422, p)
		return
	}
	if err := a.store.DeletePost(r.Context(), state(r).User.ID, item.ID); err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/history", 303)
}
