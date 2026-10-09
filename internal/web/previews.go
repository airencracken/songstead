// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/airencracken/songstead/internal/media"
)

func browseURL(p page, field, value string) string {
	path := "/shelf"
	if p.Recent {
		path = "/recent"
	}
	if p.History {
		path = "/history"
	}
	query := url.Values{"layout": {p.Layout}, "genre": {p.Genre}, "tag": {p.Tag}, "discovery": {p.Discovery}, "view": {p.Perspective}, "kind": {p.Kind}, "person": {strconv.FormatInt(p.Person, 10)}, "group": {strconv.FormatInt(p.GroupID, 10)}, "status": {p.Status}}
	query.Set(field, value)
	return path + "?" + query.Encode()
}

func (a *App) allowPreview(w http.ResponseWriter, r *http.Request) bool {
	key := "preview:" + strconv.FormatInt(state(r).User.ID, 10)
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for key, value := range a.attempts {
		if !now.Before(value.expires) {
			delete(a.attempts, key)
		}
	}
	v := a.attempts[key]
	if v.count >= 6 {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Wait a minute before requesting another preview.", 429)
		return false
	}
	if v.count == 0 {
		v.expires = now.Add(time.Minute)
	}
	v.count++
	a.attempts[key] = v
	return true
}

func (a *App) musicPreview(w http.ResponseWriter, r *http.Request) {
	raw := r.PostForm.Get("url")
	if _, err := media.Parse(raw); err != nil {
		http.Error(w, "Invalid music link.", 422)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if !media.SupportsMetadata(raw) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "unsupported"})
		return
	}
	if !a.allowPreview(w, r) {
		return
	}
	meta, err := a.previewFetch(r.Context(), raw)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "unavailable"})
		return
	}
	_ = json.NewEncoder(w).Encode(struct {
		Status  string `json:"status"`
		Title   string `json:"title"`
		Artist  string `json:"artist"`
		Artwork []byte `json:"artwork,omitempty"`
	}{"ready", meta.Title, meta.Artist, meta.Artwork})
}

func (a *App) savedPreview(w http.ResponseWriter, r *http.Request) {
	id, err := recommendationID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	item, err := a.store.Recommendation(r.Context(), state(r).User.ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	pending, err := a.store.PreviewPending(r.Context(), state(r).User.ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	image := ""
	if item.HasArtwork {
		image = "/recommendations/" + strconv.FormatInt(id, 10) + "/thumbnail"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Title   string `json:"title"`
		Artist  string `json:"artist"`
		Image   string `json:"image"`
		Pending bool   `json:"pending"`
	}{item.Title, item.Artist, image, pending})
}

func (a *App) retryPreview(w http.ResponseWriter, r *http.Request) {
	id, err := recommendationID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if !a.allowPreview(w, r) {
		return
	}
	if err = a.store.RetryPreview(r.Context(), state(r).User.ID, id); err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/recommendations/"+strconv.FormatInt(id, 10), 303)
}
