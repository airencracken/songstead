// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"fmt"
	"github.com/airencracken/comfylib/clientip"
	sharedoidc "github.com/airencracken/comfylib/oidc"
	"github.com/airencracken/comfylib/suite"
	"github.com/airencracken/comfylib/token"
	"net/http"
)

func (a *App) registerSharedSignIn(mux *http.ServeMux) error {
	auth, err := sharedoidc.FromEnvironment("SONGSTEAD_", a.config.BaseURL, a.store.DB(), suite.AuthHooks{
		RateKey: func(r *http.Request) string {
			return clientip.NetworkKey((clientip.Resolver{Trusted: a.config.TrustedProxies}).Client(r))
		},
		Current: func(r *http.Request) (int64, string) {
			u := state(r).User
			if u == nil {
				return 0, ""
			}
			binding := ""
			if c, err := r.Cookie(a.cookieName(r, "session")); err == nil {
				binding = c.Value
			}
			return u.ID, binding
		},
		CSRF: func(r *http.Request) (string, string) { return "csrf", state(r).CSRF },
		SignIn: func(w http.ResponseWriter, r *http.Request, id int64) (string, error) {
			var name string
			if err := a.store.DB().QueryRowContext(r.Context(), "SELECT username FROM users WHERE id=? AND suspended=0", id).Scan(&name); err != nil {
				return "", err
			}
			u, hash, err := a.store.Credentials(r.Context(), name)
			if err != nil || u.Suspended {
				return "", fmt.Errorf("account unavailable")
			}
			raw, err := a.store.NewSession(r.Context(), id, hash)
			if err != nil {
				return "", err
			}
			a.cookie(w, r, "session", raw, 7*86400)
			a.cookie(w, r, "csrf", token.SessionCSRF(raw, "songstead-csrf-v1"), 86400)
			return "/shelf", nil
		},
	})
	if err != nil {
		return err
	}
	if auth != nil {
		auth.Register(mux)
	}
	return nil
}
