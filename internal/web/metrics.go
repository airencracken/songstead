// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"github.com/airencracken/comfylib/admin"
	"net/http"
	"time"
)

func (a *App) metrics(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Cache-Control", "no-store")
	metrics, err := a.store.Metrics(r.Context(), time.Now())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.render(w, r, 200, page{View: "metrics", Title: "Instance metrics", Overview: admin.View{Metrics: metrics}})
}
