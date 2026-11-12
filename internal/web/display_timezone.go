package web

import (
	"errors"
	"github.com/airencracken/comfylib/displaytime"
	"net/http"
)

func preferredTimezone(r *http.Request) string {
	if r == nil {
		return "UTC"
	}
	u := state(r).User
	if u == nil {
		return "UTC"
	}
	return displaytime.New(u.Timezone).Name()
}
func timezoneControl(r *http.Request, draft *string, problem, notice string) displaytime.Preference {
	name := preferredTimezone(r)
	if draft != nil {
		name = *draft
	}
	if r.URL.Query().Get("saved") == "timezone" {
		notice = "Your display timezone has been saved."
	}
	return displaytime.Preference{Action: "/account/timezone", CSRF: state(r).CSRF, Name: name, Error: problem, Notice: notice}
}
func (a *App) saveTimezone(w http.ResponseWriter, r *http.Request) {
	name, err := displaytime.ParseForm(r.PostForm)
	if err == nil {
		err = a.store.SetTimezone(r.Context(), state(r).User.ID, name)
	}
	if errors.Is(err, displaytime.ErrTimezone) {
		a.showAccount(w, r, 422, page{TimezoneInput: &name, TimezoneError: err.Error()})
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/account?saved=timezone#display-timezone", 303)
}
