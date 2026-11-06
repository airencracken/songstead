// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"github.com/airencracken/comfylib/setup"
	"github.com/airencracken/songstead/internal/store"
)

func firstSetup(s *store.Store, data, base, addr string) (*setup.Handler, error) {
	return setup.New(setup.Config{App: "songstead", Directory: data, BaseURL: base, Address: addr, Ready: s.HasOwner, Create: s.CreateFirstOwner})
}
