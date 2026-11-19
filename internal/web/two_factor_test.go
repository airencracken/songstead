// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"strings"
	"testing"

	"github.com/airencracken/comfylib/twofactor/factortest"
)

func TestAuthenticatorSignIn(t *testing.T) {
	t.Setenv("SONGSTEAD_TOTP_KEY", strings.Repeat("ab", 32))
	app, _ := fixture(t)
	factortest.Run(t, factortest.Config{
		Handler: app, Origin: "http://example.test",
		Username: "alice", Password: "a-long-test-password", Home: "/shelf",
	})
}
