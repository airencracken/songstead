// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"errors"
	"io"

	"github.com/airencracken/comfylib/password"
	"os"

	"github.com/airencracken/songstead/internal/store"
	"golang.org/x/term"
)

func promptPassword(in io.Reader, out io.Writer) (string, error) {
	file, ok := in.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return "", errors.New("password prompt requires a terminal; use --password-stdin")
	}
	return password.WithHiddenInput(int(file.Fd()), func() (string, error) {
		return readConfirmedPassword(out, func() ([]byte, error) {
			return term.ReadPassword(int(file.Fd()))
		})
	})
}

func readConfirmedPassword(out io.Writer, read func() ([]byte, error)) (string, error) {
	value, err := password.Confirm(out, "Password: ", "Confirm password: ", read)
	if err != nil {
		return "", err
	}
	if err := store.ValidatePassword(value); err != nil {
		return "", err
	}
	return value, nil
}
