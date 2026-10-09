// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"database/sql"
	"errors"
	"github.com/airencracken/comfylib/brandimage"
	"github.com/airencracken/songstead/internal/store"
	"golang.org/x/crypto/bcrypt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (a *App) adminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin", a.owner(a.settingsForm))
	mux.HandleFunc("GET /admin/settings", a.owner(a.settingsForm))
	mux.HandleFunc("POST /admin/settings", a.owner(a.saveSettings))
	mux.HandleFunc("POST /admin/settings/reset", a.owner(a.resetSettings))
	mux.HandleFunc("POST /admin/settings/images", a.owner(a.saveImages))
	mux.HandleFunc("GET /branding/{name}", a.brandingAsset)
	mux.HandleFunc("GET /admin/users", a.owner(a.accounts))
	mux.HandleFunc("POST /admin/users/{id}", a.owner(a.changeAccount))
	mux.HandleFunc("POST /admin/users/{id}/reset", a.owner(a.createReset))
	mux.HandleFunc("POST /admin/users/{id}/reset/cancel", a.owner(a.cancelReset))
	for _, base := range []string{"/invites", "/admin/invites"} {
		mux.HandleFunc("GET "+base, a.inviter(a.invites))
		mux.HandleFunc("POST "+base, a.inviter(a.createInvite))
		mux.HandleFunc("POST "+base+"/{id}/revoke", a.inviter(a.revokeInvite))
	}
	mux.HandleFunc("GET /join", a.joinForm)
	mux.HandleFunc("POST /join", a.join)
	mux.HandleFunc("GET /reset", a.resetForm)
	mux.HandleFunc("POST /reset", a.resetPassword)
	mux.HandleFunc("GET /account", a.signedIn(a.account))
	mux.HandleFunc("POST /account/password", a.signedIn(a.changePassword))
	mux.HandleFunc("POST /account/annotations", a.signedIn(a.accountAnnotations))
	mux.HandleFunc("POST /account/discovery", a.signedIn(a.accountDiscovery))
	mux.HandleFunc("GET /about", a.about)
}
func (a *App) owner(next http.HandlerFunc) http.HandlerFunc {
	return a.signedIn(func(w http.ResponseWriter, r *http.Request) {
		if state(r).User.Role != "owner" {
			http.Error(w, "permission denied", 403)
			return
		}
		next(w, r)
	})
}
func (a *App) inviter(next http.HandlerFunc) http.HandlerFunc {
	return a.signedIn(func(w http.ResponseWriter, r *http.Request) {
		u := state(r).User
		if u.Role != "owner" && !u.CanInvite {
			http.Error(w, "permission denied", 403)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/admin/") && u.Role != "owner" {
			http.Error(w, "permission denied", 403)
			return
		}
		next(w, r)
	})
}
func (a *App) settingsForm(w http.ResponseWriter, r *http.Request) {
	p := page{View: "settings", Title: "Instance settings"}
	if r.URL.Query().Get("saved") == "1" {
		p.Notice = "Settings saved. Changes are already in effect."
	}
	a.render(w, r, 200, p)
}
func (a *App) saveSettings(w http.ResponseWriter, r *http.Request) {
	v := store.Settings{Name: strings.TrimSpace(r.PostForm.Get("name")), WelcomeTitle: strings.TrimSpace(r.PostForm.Get("welcome_title")), WelcomeText: r.PostForm.Get("welcome_text"), HouseRules: r.PostForm.Get("house_rules"), OwnerContact: r.PostForm.Get("owner_contact"), SourceURL: strings.TrimSpace(r.PostForm.Get("source_url")), BaseURL: strings.TrimSpace(r.PostForm.Get("base_url")), WitmootURL: strings.TrimSpace(r.PostForm.Get("witmoot_url")), JoinMode: r.PostForm.Get("join_mode"), DiscussionMode: r.PostForm.Get("discussion_mode"), ShowVersion: r.PostForm.Get("show_version") == "1"}
	// Older forms preserve the current policy. An explicitly empty or repeated
	// field is invalid, rather than silently changing where comments go.
	if values, present := r.PostForm["discussion_mode"]; !present {
		v.DiscussionMode = state(r).Settings.DiscussionMode
	} else if len(values) != 1 {
		v.DiscussionMode = "invalid"
	}
	if err := a.store.SaveSettings(r.Context(), state(r).User.ID, &v); err != nil {
		if errors.Is(err, store.ErrInvalid) {
			a.render(w, r, 422, page{View: "settings", Title: "Instance settings", SettingsDraft: &v, Error: "Check the field lengths, joining mode, discussion location and HTTP(S) URLs. Witmoot or Both requires both app addresses. The Songstead address must be an origin without a path; URLs cannot contain credentials, a query or a fragment."})
			return
		}
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/settings?saved=1", 303)
}
func (a *App) resetSettings(w http.ResponseWriter, r *http.Request) {
	if err := a.store.SaveSettings(r.Context(), state(r).User.ID, nil); err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/settings?saved=1", 303)
}
func (a *App) accounts(w http.ResponseWriter, r *http.Request) { a.showAccounts(w, r, 200, "") }
func (a *App) showAccounts(w http.ResponseWriter, r *http.Request, status int, secret string) {
	users, err := a.store.Accounts(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.render(w, r, status, page{View: "accounts", Title: "Accounts", Users: users, SecretURL: secret})
}
func targetID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, store.ErrInvalid
	}
	return id, nil
}
func (a *App) changeAccount(w http.ResponseWriter, r *http.Request) {
	id, err := targetID(r)
	if err == nil {
		err = a.store.ChangeAccount(r.Context(), state(r).User.ID, id, r.PostForm.Get("action"), r.PostForm.Get("value"))
	}
	if errors.Is(err, store.ErrInvalid) {
		users, loadErr := a.store.Accounts(r.Context())
		if loadErr != nil {
			a.fail(w, r, loadErr)
			return
		}
		a.render(w, r, 422, page{View: "accounts", Title: "Accounts", Users: users, Error: "Choose a valid change. At least one active owner must remain."})
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/users", 303)
}
func (a *App) link(r *http.Request, path, key, raw string) string {
	base := strings.TrimRight(state(r).Settings.BaseURL, "/")
	if base == "" {
		scheme := "http"
		if r.TLS != nil || a.secure(r) {
			scheme = "https"
		}
		u, err := url.Parse(scheme + "://" + r.Host)
		if err == nil && u.Hostname() != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" {
			base = u.String()
		}
	}
	return base + path + "?" + url.Values{key: {raw}}.Encode()
}
func (a *App) createReset(w http.ResponseWriter, r *http.Request) {
	id, err := targetID(r)
	var raw string
	if err == nil {
		raw, err = a.store.CreatePasswordReset(r.Context(), state(r).User.ID, id)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.showAccounts(w, r, 200, a.link(r, "/reset", "token", raw))
}
func (a *App) cancelReset(w http.ResponseWriter, r *http.Request) {
	id, err := targetID(r)
	if err == nil {
		err = a.store.CancelPasswordReset(r.Context(), state(r).User.ID, id)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/users", 303)
}
func (a *App) invites(w http.ResponseWriter, r *http.Request) { a.showInvites(w, r, 200, "", "") }
func (a *App) showInvites(w http.ResponseWriter, r *http.Request, status int, secret, message string) {
	offset, err := parseOptionalID(r.URL.Query().Get("offset"))
	if err != nil || offset > 100000 {
		a.fail(w, r, store.ErrInvalid)
		return
	}
	invites, err := a.store.Invitations(r.Context(), state(r).User.ID, int(offset))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	more := len(invites) > 50
	if more {
		invites = invites[:50]
	}
	a.render(w, r, status, page{View: "invites", Title: "Invitations", Invitations: invites, SecretURL: secret, Error: message, Offset: int(offset), Previous: max(0, int(offset)-50), Next: int(offset) + 50, HasPrevious: offset > 0, HasNext: more})
}
func (a *App) createInvite(w http.ResponseWriter, r *http.Request) {
	uses, e1 := strconv.Atoi(r.PostForm.Get("max_uses"))
	days, e2 := strconv.Atoi(r.PostForm.Get("days"))
	if e1 != nil || e2 != nil {
		a.showInvites(w, r, 422, "", "Choose valid use and expiry limits.")
		return
	}
	raw, err := a.store.CreateInvitation(r.Context(), state(r).User.ID, r.PostForm.Get("label"), uses, days)
	if errors.Is(err, store.ErrInvalid) {
		a.showInvites(w, r, 422, "", "Invites need an open joining mode, a label up to 64 characters, 0–10,000 uses and 0–3,650 days. Zero means unlimited.")
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.showInvites(w, r, 200, a.link(r, "/join", "invite", raw), "")
}
func (a *App) revokeInvite(w http.ResponseWriter, r *http.Request) {
	id, err := targetID(r)
	if err == nil {
		err = a.store.RevokeInvitation(r.Context(), state(r).User.ID, id)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/invites", 303)
}
func (a *App) joinForm(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("invite")
	mode := state(r).Settings.JoinMode
	if mode == "closed" || (raw == "" && mode != "open") || (raw != "" && a.store.CheckInvitation(r.Context(), raw) != nil) {
		a.render(w, r, 422, page{View: "join", Title: "Join", Error: "This invitation is unavailable, or joining is closed."})
		return
	}
	a.render(w, r, 200, page{View: "join", Title: "Join", Secret: raw, Status: "available"})
}
func (a *App) limited(w http.ResponseWriter, r *http.Request) bool {
	if a.allowLogin(r) {
		return false
	}
	w.Header().Set("Retry-After", "900")
	http.Error(w, "too many account attempts; try again in 15 minutes", 429)
	return true
}
func (a *App) join(w http.ResponseWriter, r *http.Request) {
	if a.limited(w, r) {
		return
	}
	raw := r.PostForm.Get("invite")
	name := strings.TrimSpace(r.PostForm.Get("username"))
	password := r.PostForm.Get("password")
	if password != r.PostForm.Get("confirmation") || store.ValidateUsername(name) != nil || store.ValidatePassword(password) != nil {
		a.render(w, r, 422, page{View: "join", Title: "Join", Secret: raw, Username: name, Status: "available", Error: "Use a username of 3–24 letters, numbers, underscores or dashes, and matching passwords of at least 12 characters and at most 72 bytes."})
		return
	}
	_, err := a.store.Join(r.Context(), raw, name, password)
	if errors.Is(err, store.ErrInvalid) {
		a.render(w, r, 422, page{View: "join", Title: "Join", Secret: raw, Username: name, Status: "available", Error: "That username is taken, the invitation has expired or been used, or joining is closed."})
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/login?joined=1", 303)
}
func (a *App) resetForm(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("token")
	if err := a.store.CheckPasswordReset(r.Context(), raw); err != nil {
		if !errors.Is(err, store.ErrInvalid) {
			a.fail(w, r, err)
			return
		}
		a.render(w, r, 422, page{View: "reset", Title: "Reset password", Error: "This recovery link is unavailable. Ask an owner for a new one."})
		return
	}
	a.render(w, r, 200, page{View: "reset", Title: "Reset password", Secret: raw, Status: "available"})
}
func (a *App) resetPassword(w http.ResponseWriter, r *http.Request) {
	if a.limited(w, r) {
		return
	}
	raw := r.PostForm.Get("token")
	password := r.PostForm.Get("password")
	if password != r.PostForm.Get("confirmation") || store.ValidatePassword(password) != nil {
		a.render(w, r, 422, page{View: "reset", Title: "Reset password", Secret: raw, Status: "available", Error: "Use matching passwords of at least 12 characters and at most 72 bytes."})
		return
	}
	if err := a.store.ResetPassword(r.Context(), raw, password); err != nil {
		a.fail(w, r, err)
		return
	}
	a.cookie(w, r, "session", "", -1)
	a.cookie(w, r, "csrf", "", -1)
	http.Redirect(w, r, "/login?reset=1", 303)
}
func (a *App) account(w http.ResponseWriter, r *http.Request) {
	a.showAccount(w, r, 200, page{})
}
func (a *App) changePassword(w http.ResponseWriter, r *http.Request) {
	if a.limited(w, r) {
		return
	}
	u, hash, err := a.store.Credentials(r.Context(), state(r).User.Username)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	password := r.PostForm.Get("password")
	if u.ID != state(r).User.ID || bcrypt.CompareHashAndPassword([]byte(hash), []byte(r.PostForm.Get("current_password"))) != nil || password != r.PostForm.Get("confirmation") || store.ValidatePassword(password) != nil {
		a.showAccount(w, r, 422, page{Error: "Check your current password and use matching new passwords of at least 12 characters and at most 72 bytes."})
		return
	}
	if err := a.store.ChangePassword(r.Context(), u.ID, hash, password); err != nil {
		a.fail(w, r, err)
		return
	}
	a.cookie(w, r, "session", "", -1)
	a.cookie(w, r, "csrf", "", -1)
	http.Redirect(w, r, "/login?reset=1", 303)
}
func (a *App) about(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, 200, page{View: "about", Title: "About this gathering"})
}
func uploadedImage(r *http.Request, name string) ([]byte, error) {
	f, h, err := r.FormFile(name)
	if errors.Is(err, http.ErrMissingFile) {
		return nil, nil
	}
	if err != nil {
		return nil, store.ErrInvalid
	}
	defer f.Close()
	if h.Size > brandimage.MaxBytes {
		return nil, store.ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(f, brandimage.MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > brandimage.MaxBytes {
		return nil, store.ErrInvalid
	}
	return data, nil
}
func (a *App) saveImages(w http.ResponseWriter, r *http.Request) {
	mascot, e1 := uploadedImage(r, "mascot")
	favicon, e2 := uploadedImage(r, "favicon")
	removeMascot := r.PostForm.Get("remove_mascot") == "1"
	removeFavicon := r.PostForm.Get("remove_favicon") == "1"
	if e1 != nil || e2 != nil || (len(mascot) == 0 && len(favicon) == 0 && !removeMascot && !removeFavicon) {
		a.render(w, r, 422, page{View: "settings", Title: "Instance settings", Error: "Choose a PNG, JPEG or GIF up to 2 MiB and 2048 by 2048 pixels, or select an image to remove."})
		return
	}
	err := a.store.SaveBrandingAssets(r.Context(), state(r).User.ID, mascot, favicon, removeMascot, removeFavicon)
	if errors.Is(err, store.ErrInvalid) {
		a.render(w, r, 422, page{View: "settings", Title: "Instance settings", Error: "Images must be valid PNG, JPEG or GIF, up to 2 MiB and 2048 by 2048 pixels. Neither image was changed."})
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/settings?saved=1", 303)
}
func (a *App) brandingAsset(w http.ResponseWriter, r *http.Request) {
	content, err := a.store.BrandingAsset(r.Context(), r.PathValue("name"))
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	if r.Method != "HEAD" {
		_, _ = w.Write(content)
	}
}
