// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"errors"
	"github.com/airencracken/comfylib/reference"
	"github.com/airencracken/songstead/internal/annotations"
	"github.com/airencracken/songstead/internal/store"
	"net/http"
	"strconv"
	"strings"
)

func parseOptionalID(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, store.ErrInvalid
	}
	return n, nil
}
func (a *App) groups(w http.ResponseWriter, r *http.Request) {
	groups, err := a.store.Groups(r.Context(), state(r).User.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	for i := range groups {
		members, err := a.store.GroupMembers(r.Context(), state(r).User.ID, groups[i].ID)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		groups[i].Members = map[int64]bool{}
		for _, id := range members {
			groups[i].Members[id] = true
		}
	}
	users, err := a.store.Users(r.Context(), 0)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.render(w, r, 200, page{View: "groups", Title: "Your groups", Groups: groups, Users: users})
}
func (a *App) saveGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseOptionalID(r.PostForm.Get("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	members := []int64{}
	for key, values := range r.PostForm {
		if strings.HasPrefix(key, "member_") {
			n, err := parseOptionalID(strings.TrimPrefix(key, "member_"))
			if err != nil || n == 0 || len(values) != 1 || values[0] != "1" {
				a.fail(w, r, store.ErrInvalid)
				return
			}
			members = append(members, n)
		}
	}
	for _, raw := range strings.Split(r.PostForm.Get("members"), ",") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		n, err := parseOptionalID(strings.TrimSpace(raw))
		if err != nil || n == 0 {
			a.fail(w, r, store.ErrInvalid)
			return
		}
		members = append(members, n)
	}
	_, err = a.store.SaveGroup(r.Context(), state(r).User.ID, id, r.PostForm.Get("name"), members)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/groups", 303)
}
func (a *App) finishDetail(w http.ResponseWriter, r *http.Request, id int64, err error) {
	if errors.Is(err, store.ErrInvalid) {
		a.showDetail(w, r, id, 422, "Check the recording, position, or discussion address.")
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/recommendations/"+strconv.FormatInt(id, 10), 303)
}
func (a *App) addRecording(w http.ResponseWriter, r *http.Request) {
	id, err := recommendationID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	duration := 0
	if raw := r.PostForm.Get("duration"); raw != "" {
		var ok bool
		duration, ok = annotations.Seconds(raw)
		if !ok {
			a.finishDetail(w, r, id, store.ErrInvalid)
			return
		}
	}
	err = a.store.AddRecording(r.Context(), state(r).User.ID, id, r.PostForm.Get("url"), r.PostForm.Get("title"), duration)
	a.finishDetail(w, r, id, err)
}
func (a *App) position(w http.ResponseWriter, r *http.Request) {
	id, err := recommendationID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	track, err := parseOptionalID(r.PostForm.Get("recording"))
	seconds, ok := annotations.Seconds(r.PostForm.Get("position"))
	if err != nil || !ok {
		a.finishDetail(w, r, id, store.ErrInvalid)
		return
	}
	err = a.store.SetPosition(r.Context(), state(r).User.ID, id, track, seconds)
	a.finishDetail(w, r, id, err)
}
func (a *App) annotationPreference(w http.ResponseWriter, r *http.Request) {
	id, err := recommendationID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if _, err = a.store.Recommendation(r.Context(), state(r).User.ID, id); err == nil {
		err = a.store.SetAnnotationMode(r.Context(), state(r).User.ID, r.PostForm.Get("mode"))
	}
	a.finishDetail(w, r, id, err)
}
func (a *App) discussion(w http.ResponseWriter, r *http.Request) {
	id, err := recommendationID(r)
	if err == nil {
		if r.PostForm.Get("remove") == "1" {
			err = a.store.RemoveDiscussion(r.Context(), state(r).User.ID, id, r.PostForm.Get("url"))
		} else {
			err = a.store.LinkDiscussion(r.Context(), state(r).User.ID, id, r.PostForm.Get("url"))
		}
	}
	a.finishDetail(w, r, id, err)
}
func (a *App) share(w http.ResponseWriter, r *http.Request) {
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
	if state(r).Settings.WitmootURL == "" || state(r).Settings.BaseURL == "" {
		a.showDetail(w, r, id, 422, "The operator needs to configure the Songstead and Witmoot addresses.")
		return
	}
	source := strings.TrimRight(state(r).Settings.BaseURL, "/") + "/recommendations/" + strconv.FormatInt(id, 10)
	draft := reference.Draft{Source: source, Title: item.Title, Body: item.Title}
	if item.Artist != "" {
		draft.Body += " — " + item.Artist
	}
	draft.Body += "\n" + item.URL
	link, err := reference.Handoff(state(r).Settings.WitmootURL, draft)
	if err != nil {
		a.fail(w, r, store.ErrInvalid)
		return
	}
	a.render(w, r, 200, page{View: "handoff", Title: "A discussion, if you like", Handoff: link})
}
