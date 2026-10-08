// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/airencracken/songstead/internal/store"
	"golang.org/x/term"
)

func promptPassword(in io.Reader, out io.Writer) (string, error) {
	file, ok := in.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return "", errors.New("password prompt requires a terminal; use --password-stdin")
	}
	return readConfirmedPassword(out, func() ([]byte, error) {
		return term.ReadPassword(int(file.Fd()))
	})
}

func readConfirmedPassword(out io.Writer, read func() ([]byte, error)) (string, error) {
	password, err := readPromptLine(out, "Password: ", read)
	if err != nil {
		return "", err
	}
	confirmation, err := readPromptLine(out, "Confirm password: ", read)
	if err != nil {
		return "", err
	}
	if string(password) != string(confirmation) {
		return "", errors.New("passwords do not match")
	}
	value := string(password)
	if err := store.ValidatePassword(value); err != nil {
		return "", err
	}
	return value, nil
}

func readPromptLine(out io.Writer, prompt string, read func() ([]byte, error)) ([]byte, error) {
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return nil, err
	}
	password, err := read()
	if _, writeErr := fmt.Fprintln(out); err == nil {
		err = writeErr
	}
	return password, err
}
