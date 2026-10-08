// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/airencracken/comfylib/token"
	"golang.org/x/crypto/bcrypt"
	"time"
)

type Invitation struct {
	ID, CreatorID, CreatedAt, ExpiresAt int64
	Prefix, Label, Creator              string
	MaxUses, Uses                       int
	Revoked                             bool
}

func inviteAllowed(ctx context.Context, tx *sql.Tx, actor int64) error {
	var ok bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id=? AND suspended=0 AND (role='owner' OR can_invite=1))", actor).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}
func (s *Store) CreateInvitation(ctx context.Context, actor int64, label string, maxUses, days int) (string, error) {
	if !validText(label, 0, 64) || maxUses < 0 || maxUses > 10000 || days < 0 || days > 3650 {
		return "", ErrInvalid
	}
	secret := token.NewPrefixed("sgi")
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if err := inviteAllowed(ctx, tx, actor); err != nil {
		return "", err
	}
	settings, err := readSettings(ctx, tx, DefaultSettings("", ""))
	if err != nil {
		return "", err
	}
	if settings.JoinMode == "closed" {
		return "", ErrInvalid
	}
	expiry := int64(0)
	if days > 0 {
		expiry = time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO invitations(token_hash,prefix,creator_id,label,created_at,expires_at,max_uses) VALUES(?,?,?,?,?,?,?)", secret.Hash, secret.Prefix, actor, label, time.Now().Unix(), expiry, maxUses)
	if err != nil {
		return "", err
	}
	return secret.Full, tx.Commit()
}
func (s *Store) Invitations(ctx context.Context, actor int64, offset int) ([]Invitation, error) {
	if offset < 0 {
		return nil, ErrInvalid
	}
	rows, err := s.db.QueryContext(ctx, `SELECT i.id,i.creator_id,i.prefix,i.label,u.username,i.created_at,i.expires_at,i.max_uses,i.uses,i.revoked FROM invitations i JOIN users u ON u.id=i.creator_id
 WHERE EXISTS(SELECT 1 FROM users a WHERE a.id=? AND a.suspended=0 AND (a.role='owner' OR (a.can_invite=1 AND i.creator_id=a.id))) ORDER BY i.id DESC LIMIT 51 OFFSET ?`, actor, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Invitation{}
	for rows.Next() {
		var i Invitation
		if err := rows.Scan(&i.ID, &i.CreatorID, &i.Prefix, &i.Label, &i.Creator, &i.CreatedAt, &i.ExpiresAt, &i.MaxUses, &i.Uses, &i.Revoked); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (s *Store) RevokeInvitation(ctx context.Context, actor, id int64) error {
	return mutationResult(s.db.ExecContext(ctx, `UPDATE invitations SET revoked=1 WHERE id=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND suspended=0 AND (role='owner' OR (can_invite=1 AND invitations.creator_id=users.id)))`, id, actor))
}

const usableInvitation = `SELECT i.id,i.creator_id FROM invitations i JOIN users u ON u.id=i.creator_id WHERE i.token_hash=? AND i.revoked=0 AND (i.expires_at=0 OR i.expires_at>?) AND (i.max_uses=0 OR i.uses<i.max_uses) AND u.suspended=0 AND (u.role='owner' OR u.can_invite=1)`

func checkInvitation(ctx context.Context, db settingsReader, raw string) (id, creator int64, err error) {
	if _, ok := token.SplitPrefixed("sgi", raw); !ok {
		return 0, 0, ErrInvalid
	}
	err = db.QueryRowContext(ctx, usableInvitation, token.Hash(raw), time.Now().Unix()).Scan(&id, &creator)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrInvalid
	}
	return
}
func (s *Store) CheckInvitation(ctx context.Context, raw string) error {
	_, _, err := checkInvitation(ctx, s.db, raw)
	return err
}
func (s *Store) Join(ctx context.Context, raw, name, password string) (int64, error) {
	if ValidateUsername(name) != nil || ValidatePassword(password) != nil {
		return 0, ErrInvalid
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	settings, err := readSettings(ctx, tx, DefaultSettings("", ""))
	if err != nil {
		return 0, err
	}
	if settings.JoinMode == "closed" || (raw == "" && settings.JoinMode != "open") {
		return 0, ErrInvalid
	}
	var invitation, creator int64
	var invitedBy any
	if raw != "" {
		invitation, creator, err = checkInvitation(ctx, tx, raw)
		if err != nil {
			return 0, err
		}
		invitedBy = creator
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE username=?)", name).Scan(&exists); err != nil {
		return 0, err
	}
	if exists {
		return 0, ErrInvalid
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO users(username,password_hash,created_at,invited_by) VALUES(?,?,?,?)", name, string(hash), time.Now().Unix(), invitedBy)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if invitation != 0 {
		if _, err := tx.ExecContext(ctx, "UPDATE invitations SET uses=uses+1 WHERE id=?", invitation); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}
func (s *Store) Accounts(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT u.id,u.username,u.role,u.can_invite,u.suspended,coalesce(inviter.username,'') FROM users u LEFT JOIN users inviter ON inviter.id=u.invited_by ORDER BY u.username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CanInvite, &u.Suspended, &u.InvitedBy); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ChangeAccount rechecks the actor and last active owner inside the same write
// transaction. Permission removal invalidates invitations rather than pausing them.
func (s *Store) ChangeAccount(ctx context.Context, actor, target int64, action, value string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireOwner(ctx, tx, actor); err != nil {
		return err
	}
	if err := changeAccount(ctx, tx, target, action, value); err != nil {
		return err
	}
	return tx.Commit()
}
func changeAccount(ctx context.Context, tx *sql.Tx, target int64, action, value string) error {
	var u User
	err := tx.QueryRowContext(ctx, "SELECT role,suspended FROM users WHERE id=?", target).Scan(&u.Role, &u.Suspended)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMissing
	}
	if err != nil {
		return err
	}
	var column string
	switch action {
	case "role":
		if value != "owner" && value != "member" {
			return ErrInvalid
		}
		column = "role"
	case "suspended", "can_invite":
		if value != "0" && value != "1" {
			return ErrInvalid
		}
		column = action
	default:
		return ErrInvalid
	}
	if u.Role == "owner" && !u.Suspended && ((action == "role" && value == "member") || (action == "suspended" && value == "1")) {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE role='owner' AND suspended=0").Scan(&count); err != nil {
			return err
		}
		if count <= 1 {
			return ErrInvalid
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE users SET "+column+"=? WHERE id=?", value, target); err != nil {
		return err
	}
	if action == "role" || (action == "can_invite" && value == "0") || (action == "suspended" && value == "1") {
		if _, err := tx.ExecContext(ctx, "UPDATE invitations SET revoked=1 WHERE creator_id=?", target); err != nil {
			return err
		}
	}
	if action == "suspended" && value == "1" {
		if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=?", target); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM password_resets WHERE user_id=?", target); err != nil {
			return err
		}
	}
	return nil
}

// SetRole is an explicit local recovery operation; it never guesses which
// existing account should become owner and still protects the last active owner.
func (s *Store) SetRole(ctx context.Context, name, role string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM users WHERE username=?", name).Scan(&id); err != nil {
		return err
	}
	if err := changeAccount(ctx, tx, id, "role", role); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) CreatePasswordReset(ctx context.Context, actor, target int64) (string, error) {
	secret := token.NewPrefixed("sgr")
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if err := requireOwner(ctx, tx, actor); err != nil {
		return "", err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO password_resets(user_id,token_hash,password_hash,expires_at) SELECT id,?,password_hash,? FROM users WHERE id=? AND suspended=0
 ON CONFLICT(user_id) DO UPDATE SET token_hash=excluded.token_hash,password_hash=excluded.password_hash,expires_at=excluded.expires_at`, secret.Hash, time.Now().Add(time.Hour).Unix(), target)
	if err := mutationResult(result, err); err != nil {
		return "", err
	}
	return secret.Full, tx.Commit()
}
func (s *Store) CancelPasswordReset(ctx context.Context, actor, target int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireOwner(ctx, tx, actor); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM password_resets WHERE user_id=?", target); err != nil {
		return err
	}
	return tx.Commit()
}

const usableReset = `SELECT u.id FROM password_resets r JOIN users u ON u.id=r.user_id WHERE r.token_hash=? AND r.expires_at>? AND r.password_hash=u.password_hash AND u.suspended=0`

func checkReset(ctx context.Context, db settingsReader, raw string) (int64, error) {
	if _, ok := token.SplitPrefixed("sgr", raw); !ok {
		return 0, ErrInvalid
	}
	var id int64
	err := db.QueryRowContext(ctx, usableReset, token.Hash(raw), time.Now().Unix()).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrInvalid
	}
	return id, err
}
func (s *Store) CheckPasswordReset(ctx context.Context, raw string) error {
	_, err := checkReset(ctx, s.db, raw)
	return err
}
func (s *Store) ResetPassword(ctx context.Context, raw, password string) error {
	if ValidatePassword(password) != nil {
		return ErrInvalid
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	id, err := checkReset(ctx, tx, raw)
	if err != nil {
		return err
	}
	if err := replacePassword(ctx, tx, id, string(hash)); err != nil {
		return err
	}
	return tx.Commit()
}
func replacePassword(ctx context.Context, tx *sql.Tx, id int64, hash string) error {
	if _, err := tx.ExecContext(ctx, "UPDATE users SET password_hash=? WHERE id=?", hash, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM password_resets WHERE user_id=?", id); err != nil {
		return err
	}
	return nil
}

func (s *Store) ChangePassword(ctx context.Context, id int64, verifiedHash, password string) error {
	if ValidatePassword(password) != nil {
		return ErrInvalid
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var ok bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id=? AND password_hash=? AND suspended=0)", id, verifiedHash).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return ErrInvalid
	}
	if err := replacePassword(ctx, tx, id, string(hash)); err != nil {
		return err
	}
	return tx.Commit()
}
