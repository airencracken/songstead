// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"errors"
	"net/http"

	"github.com/airencracken/comfylib/problem"
)

// Songstead-specific messages. Other statuses use Comfylib's plain defaults.
const (
	problemForm    = "That form couldn't be read. Reload the page and try again."
	problemView    = "That view isn't available. Go back to your shelf and choose another."
	problemSignIns = "Too many sign-in attempts. Try again in 15 minutes."
)

var errNoRequestState = errors.New("request failed before the session loaded")

// problemLayout renders a problem inside Songstead's layout. Requests that
// fail before the session and settings load get Comfylib's minimal page.
func (a *App) problemLayout(w http.ResponseWriter, r *http.Request, shown problem.Page) error {
	if _, ok := r.Context().Value(stateKey{}).(requestState); !ok {
		return errNoRequestState
	}
	return a.renderPage(w, r, shown.Status, page{View: "problem", Title: shown.Title, Problem: shown.Message})
}

// problem writes a styled error page. An empty message uses the default for
// status.
func (a *App) problem(w http.ResponseWriter, r *http.Request, status int, message string) {
	a.problems.Write(w, r, status, message, nil)
}
