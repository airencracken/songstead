// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/airencracken/comfylib/memberprofile"
)

// MemberProfile deliberately omits account credentials and private preferences.
type MemberProfile struct {
	ID        int64
	Username  string
	Biography memberprofile.Profile
}

func readMemberProfile(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64) (MemberProfile, error) {
	var p MemberProfile
	var links string
	err := db.QueryRowContext(ctx, `SELECT u.id,u.username,coalesce(p.name,''),coalesce(p.bio,''),coalesce(p.links,'[]') FROM users u LEFT JOIN member_profiles p ON p.user_id=u.id WHERE u.id=? AND u.suspended=0`, id).Scan(&p.ID, &p.Username, &p.Biography.Name, &p.Biography.Bio, &links)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrMissing
	}
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal([]byte(links), &p.Biography.Links); err != nil {
		return p, err
	}
	p.Biography, err = memberprofile.Normalize(p.Biography)
	return p, err
}

func (s *Store) MemberProfile(ctx context.Context, id int64) (MemberProfile, error) {
	return readMemberProfile(ctx, s.db, id)
}

// SetMemberProfile replaces all optional fields in one statement and checks the
// account is still active at the mutation boundary.
func (s *Store) SetMemberProfile(ctx context.Context, id int64, p memberprofile.Profile) error {
	p, err := memberprofile.Normalize(p)
	if err != nil {
		return errors.Join(ErrInvalid, err)
	}
	links, err := json.Marshal(p.Links)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO member_profiles(user_id,name,bio,links) SELECT id,?,?,? FROM users WHERE id=? AND suspended=0 ON CONFLICT(user_id) DO UPDATE SET name=excluded.name,bio=excluded.bio,links=excluded.links`, p.Name, p.Bio, string(links), id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		return ErrMissing
	}
	return err
}
