// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"time"

	"github.com/airencracken/comfylib/setup"
	"golang.org/x/crypto/bcrypt"
)

// HasOwner includes suspended owners so setup never reopens an existing instance.
func (s *Store) HasOwner(ctx context.Context) (bool, error) {
	var present bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE role='owner')").Scan(&present)
	return present, err
}

// CreateFirstOwner creates the browser setup account with an atomic owner guard.
func (s *Store) CreateFirstOwner(ctx context.Context, owner setup.Owner) error {
	if err := ValidateUsername(owner.Username); err != nil {
		return err
	}
	if err := ValidatePassword(owner.Password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(owner.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var present bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE role='owner')").Scan(&present); err != nil {
		return err
	}
	if present {
		return setup.ErrComplete
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO users(username,password_hash,created_at,role) VALUES(?,?,?,'owner')", owner.Username, string(hash), time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}
