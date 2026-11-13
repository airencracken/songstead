// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"net/http"
)

// Plain-language messages for requests that cannot continue.
const (
	problemForbidden = "Your account can't open this page."
	problemMissing   = "This page doesn't exist, or it isn't shared with you."
	problemInvalid   = "Something in that request didn't look right. Go back and try again."
	problemForm      = "That form couldn't be read. Reload the page and try again."
	problemTooLarge  = "That was too large to save. Try a shorter entry or a smaller image."
	problemExpired   = "This page was open for a while. Reload it and try again."
	problemView      = "That view isn't available. Go back to your shelf and choose another."
	problemSignIns   = "Too many sign-in attempts. Try again in 15 minutes."
	problemServer    = "Something went wrong. Please try again."
)

var problemTitles = map[int]string{
	http.StatusBadRequest:            "That didn't work",
	http.StatusForbidden:             "Not available",
	http.StatusNotFound:              "Nothing here",
	http.StatusRequestEntityTooLarge: "Too large",
	http.StatusUnprocessableEntity:   "That didn't work",
	http.StatusTooManyRequests:       "Take a short break",
	http.StatusInternalServerError:   "Something went wrong",
}

// problem renders a styled error page. Requests that fail before the session
// and settings load fall back to plain text.
func (a *App) problem(w http.ResponseWriter, r *http.Request, status int, message string) {
	if _, ok := r.Context().Value(stateKey{}).(requestState); !ok {
		http.Error(w, message, status)
		return
	}
	title := problemTitles[status]
	if title == "" {
		title = "That didn't work"
	}
	a.render(w, r, status, page{View: "problem", Title: title, Problem: message})
}
