// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/airencracken/comfylib/profileimage"
)

type Profile struct {
	Still     []byte `json:"still,omitempty"`
	Animation []byte `json:"animation,omitempty"`
	Animate   bool   `json:"animate"`
}

func readProfile(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, user int64) (Profile, error) {
	v := Profile{Animate: true}
	err := db.QueryRowContext(ctx, "SELECT still,animation,animate FROM user_profiles WHERE user_id=?", user).Scan(&v.Still, &v.Animation, &v.Animate)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{Animate: true}, nil
	}
	return v, err
}

func (s *Store) Profile(ctx context.Context, user int64) (Profile, error) {
	return readProfile(ctx, s.db, user)
}

// Normalization completes before the single atomic update; failures leave the
// previous still, animation and viewer preference intact. Nil removes a picture.
func (s *Store) SetPicture(ctx context.Context, user int64, data []byte) error {
	picture := profileimage.Picture{Still: []byte{}, Animation: []byte{}}
	if data != nil {
		var err error
		picture, err = profileimage.Normalize(data)
		if err != nil {
			return errors.Join(ErrInvalid, err)
		}
		if picture.Animation == nil {
			picture.Animation = []byte{}
		}
	}
	_, err := s.db.ExecContext(ctx, "INSERT INTO user_profiles(user_id,still,animation) VALUES(?,?,?) ON CONFLICT(user_id) DO UPDATE SET still=excluded.still,animation=excluded.animation", user, picture.Still, picture.Animation)
	return err
}

func (s *Store) SetAnimationPreference(ctx context.Context, user int64, animate bool) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO user_profiles(user_id,animate) VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET animate=excluded.animate", user, animate)
	return err
}

func (s *Store) Picture(ctx context.Context, viewer, user int64, still bool) ([]byte, string, error) {
	var exists bool
	if err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id=? AND suspended=0)", user).Scan(&exists); err != nil {
		return nil, "", err
	}
	if !exists {
		return nil, "", ErrMissing
	}
	v, err := s.Profile(ctx, viewer)
	if err != nil {
		return nil, "", err
	}
	p, err := s.Profile(ctx, user)
	if err != nil {
		return nil, "", err
	}
	if !still && v.Animate && len(p.Animation) > 0 {
		return p.Animation, "image/gif", nil
	}
	return p.Still, "image/png", nil
}
