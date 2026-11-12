// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"github.com/airencracken/comfylib/analytics"
	"net/http"
)

func (a *App) trackOpen(kind analytics.Kind, next http.HandlerFunc) http.HandlerFunc {
	return analytics.Track(a.store.AnalyticsDB(), next, func(r *http.Request) []analytics.Event {
		u := state(r).User
		if u == nil {
			return nil
		}
		if kind == analytics.Image {
			if r.URL.Query().Has("width") {
				return nil
			}
		}
		item := analytics.NumericID(r.PathValue("id"))
		if item == "" {
			return nil
		}
		return []analytics.Event{{Kind: kind, Member: analytics.MemberID(u.ID), Item: item}}
	})
}
