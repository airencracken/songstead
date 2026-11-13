// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestSharedSignInRecordsNoPerAccountActivity(t *testing.T) {
	a, users := fixture(t)
	request := httptest.NewRequest("GET", "http://example.test/auth/shared/callback", nil)
	if _, err := a.sharedSignIn(httptest.NewRecorder(), request, users[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.DB().Exec("UPDATE users SET suspended=1 WHERE id=?", users[0]); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	if _, err := a.sharedSignIn(response, request, users[0]); err == nil {
		t.Fatal("inactive account signed in")
	}
	if len(response.Result().Cookies()) != 0 {
		t.Fatal("inactive account received a session")
	}
	report, err := a.store.Analytics(t.Context(), time.Now())
	if err != nil || len(report.LoginsByAccount) != 0 || report.Logins != 0 {
		t.Fatal(report, err)
	}
}
