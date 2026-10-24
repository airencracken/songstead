// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"time"

	"github.com/airencracken/comfylib/invitation"
	"github.com/airencracken/songstead/internal/store"
)

func invitationStatus(i store.Invitation, now int64) string {
	var expiry *time.Time
	if i.ExpiresAt != 0 {
		value := time.Unix(i.ExpiresAt, 0)
		expiry = &value
	}
	return (invitation.State{Revoked: i.Revoked, ExpiresAt: expiry, Uses: i.Uses, MaxUses: i.MaxUses}).Status(time.Unix(now, 0)).Label()
}
