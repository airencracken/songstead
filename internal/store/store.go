// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/airencracken/comfylib/token"
	"github.com/airencracken/songstead/internal/media"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

var ErrInvalid = errors.New("invalid input")
var ErrMissing = errors.New("recommendation unavailable")

type Store struct{ db *sql.DB }
type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CanInvite bool   `json:"can_invite"`
	Suspended bool   `json:"suspended"`
	InvitedBy string `json:"invited_by,omitempty"`
}
type Recommendation struct {
	ID, MediaID, SenderID, RecipientID                                              int64
	Sender, Recipient, URL, Provider, Title, Artist, Thumbnail, Type, VideoID, Note string
	CreatedAt                                                                       int64
	GroupID                                                                         int64
	Group, Kind                                                                     string
	Visibility                                                                      string
	Listening                                                                       string
	Rating                                                                          int
	PersonalNote                                                                    string
}
type Comment struct {
	RecordingID             int64
	Offsets                 []Offset
	ID, AuthorID, CreatedAt int64
	Author, Body            string
}
type Reaction struct {
	Listening string
	Rating    int
	Note      string
}

func Open(path string) (*Store, error) {
	return open(path, false)
}

// OpenCurrent opens a provisioned instance without applying migrations while
// an older server might still be running. Start the updated server first.
func OpenCurrent(path string) (*Store, error) {
	return open(path, true)
}

func open(path string, currentOnly bool) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: abs}
	u.RawQuery = "_txlock=immediate&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(currentOnly); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(currentOnly bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 5 {
		return fmt.Errorf("database schema %d is newer than this binary", version)
	}
	if currentOnly && version != 5 {
		return fmt.Errorf("database schema %d needs migration; restart the updated Songstead server before running account or backup commands", version)
	}
	for next := version + 1; next <= 5; next++ {
		file := map[int]string{1: "migrations/001_initial.sql", 2: "migrations/002_quiet_inbox.sql", 3: "migrations/003_recent.sql", 4: "migrations/004_owners.sql", 5: "migrations/005_administration.sql"}[next]
		data, err := migrations.ReadFile(file)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(data)); err != nil {
			return err
		}
		if next == 2 {
			if err := migrateMusic(tx); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", next)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

var username = regexp.MustCompile(`^[A-Za-z0-9_-]{3,24}$`)

func ValidateUsername(name string) error {
	if !username.MatchString(name) {
		return errors.New("username needs 3-24 letters, numbers, underscores or dashes")
	}
	return nil
}

func ValidatePassword(password string) error {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 12 || len(password) > 72 {
		return errors.New("password needs at least 12 characters and at most 72 bytes")
	}
	return nil
}

func (s *Store) CreateUser(ctx context.Context, name, password string) (int64, error) {
	return s.createAccount(ctx, name, password, "member")
}

// CreateOwner provisions a new owner. Duplicate usernames, including names
// differing only in case, never promote or change an existing account.
func (s *Store) CreateOwner(ctx context.Context, name, password string) (int64, error) {
	return s.createAccount(ctx, name, password, "owner")
}

func (s *Store) createAccount(ctx context.Context, name, password, role string) (int64, error) {
	if err := ValidateUsername(name); err != nil {
		return 0, err
	}
	if err := ValidatePassword(password); err != nil {
		return 0, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	result, err := s.db.ExecContext(ctx, "INSERT INTO users(username,password_hash,created_at,role) VALUES(?,?,?,?)", name, string(hash), time.Now().Unix(), role)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) SetPassword(ctx context.Context, name, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
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
	result, err := tx.ExecContext(ctx, "UPDATE users SET password_hash=? WHERE username=?", string(hash), name)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=(SELECT id FROM users WHERE username=?)", name); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM password_resets WHERE user_id=(SELECT id FROM users WHERE username=?)", name); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Credentials(ctx context.Context, name string) (User, string, error) {
	var u User
	var hash string
	err := s.db.QueryRowContext(ctx, "SELECT id,username,role,can_invite,suspended,password_hash FROM users WHERE username=?", name).Scan(&u.ID, &u.Username, &u.Role, &u.CanInvite, &u.Suspended, &hash)
	return u, hash, err
}

func (s *Store) Users(ctx context.Context, except int64) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,username,role,can_invite,suspended FROM users WHERE id!=? AND suspended=0 ORDER BY username", except)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CanInvite, &u.Suspended); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) Session(ctx context.Context, secret string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, "SELECT u.id,u.username,u.role,u.can_invite,u.suspended FROM users u JOIN sessions s ON s.user_id=u.id WHERE s.token_hash=? AND s.expires_at>? AND u.suspended=0", token.Hash(secret), time.Now().Unix()).Scan(&u.ID, &u.Username, &u.Role, &u.CanInvite, &u.Suspended)
	return u, err
}

// NewSession checks that the credentials verified by the caller are still
// current. A concurrent password change cannot grant a session with an old hash.
func (s *Store) NewSession(ctx context.Context, user int64, verifiedHash string) (string, error) {
	secret, hash := token.New()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at<=?", time.Now().Unix()); err != nil {
		return "", err
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) SELECT ?,id,? FROM users WHERE id=? AND password_hash=? AND suspended=0", hash, time.Now().Add(7*24*time.Hour).Unix(), user, verifiedHash)
	if err != nil {
		return "", err
	}
	if err := mutationResult(result, nil); err != nil {
		return "", err
	}
	return secret, tx.Commit()
}

func (s *Store) Logout(ctx context.Context, secret string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=?", token.Hash(secret))
	return err
}

func validText(s string, min, max int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) >= min && utf8.RuneCountInString(s) <= max &&
		strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) < 0
}

func (s *Store) Recommend(ctx context.Context, sender, recipient int64, raw, note string) (int64, error) {
	return s.recommend(ctx, sender, recipient, 0, raw, note, "", "", "", "private")
}

const selectRecommendation = `SELECT r.id,m.id,r.sender_id,d.user_id,sender.username,recipient.username,
 coalesce(nullif(r.source_url,''),m.original_url),m.provider,m.title,m.artist,m.thumbnail,m.media_type,m.video_id,r.note,r.created_at,
 coalesce(ms.listening,'unheard'),coalesce(x.rating,0),coalesce(x.note,''),coalesce(r.group_id,0),coalesce(g.name,''),m.kind,r.visibility
 FROM recommendations r JOIN media m ON m.id=r.media_id
 JOIN recommendation_destinations d ON d.recommendation_id=r.id
 JOIN users sender ON sender.id=r.sender_id JOIN users recipient ON recipient.id=d.user_id
 LEFT JOIN reactions x ON x.recommendation_id=r.id AND x.user_id=?
 LEFT JOIN music_states ms ON ms.media_id=m.id AND ms.user_id=?
 LEFT JOIN groups g ON g.id=r.group_id `

// The same visibility predicate protects details, comments and mutations.
const visible = `((r.group_id IS NULL AND (r.sender_id=? OR EXISTS(SELECT 1 FROM users v WHERE v.id=? AND (r.visibility='members' OR d.user_id=v.id)))) OR (r.group_id IS NOT NULL AND EXISTS(SELECT 1 FROM group_members gm WHERE gm.group_id=r.group_id AND gm.user_id=?)))`

type scanner interface{ Scan(...any) error }

func scanRecommendation(row scanner) (Recommendation, error) {
	var r Recommendation
	err := row.Scan(&r.ID, &r.MediaID, &r.SenderID, &r.RecipientID, &r.Sender, &r.Recipient, &r.URL, &r.Provider, &r.Title, &r.Artist, &r.Thumbnail, &r.Type, &r.VideoID, &r.Note, &r.CreatedAt, &r.Listening, &r.Rating, &r.PersonalNote, &r.GroupID, &r.Group, &r.Kind, &r.Visibility)
	return r, err
}

func (s *Store) Recommendation(ctx context.Context, viewer, id int64) (Recommendation, error) {
	r, err := scanRecommendation(s.db.QueryRowContext(ctx, selectRecommendation+" WHERE r.id=? AND "+visible, viewer, viewer, id, viewer, viewer, viewer))
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrMissing
	}
	return r, err
}

func (s *Store) List(ctx context.Context, viewer int64, history bool, status string, limit, offset int) ([]Recommendation, error) {
	return s.Browse(ctx, viewer, history, Filter{Status: status}, limit, offset)
}

func validListening(s string) bool {
	return s == "saved" || s == "explored" || s == "unheard" || s == "listened" || s == "revisit" || s == "dismissed"
}

func (s *Store) React(ctx context.Context, viewer, id int64, reaction Reaction) error {
	if !validListening(reaction.Listening) || reaction.Rating < -1 || reaction.Rating > 1 || !validText(reaction.Note, 0, 2000) {
		return ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var mid int64
	err = tx.QueryRowContext(ctx, `SELECT r.media_id FROM recommendations r JOIN recommendation_destinations d ON d.recommendation_id=r.id WHERE r.id=? AND `+visible, id, viewer, viewer, viewer).Scan(&mid)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMissing
	}
	if err != nil {
		return err
	}
	legacy := reaction.Listening
	if legacy == "saved" || legacy == "explored" {
		legacy = "unheard"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO reactions VALUES(?,?,?,?,?,?) ON CONFLICT(recommendation_id,user_id) DO UPDATE SET listening=excluded.listening,rating=excluded.rating,note=excluded.note,updated_at=excluded.updated_at`, id, viewer, legacy, reaction.Rating, reaction.Note, time.Now().Unix())
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO music_states VALUES(?,?,?,?) ON CONFLICT(media_id,user_id) DO UPDATE SET listening=excluded.listening,updated_at=excluded.updated_at`, mid, viewer, reaction.Listening, time.Now().Unix())
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AddComment(ctx context.Context, viewer, id int64, body string) error {
	return s.Annotate(ctx, viewer, id, 0, body)
}

func mutationResult(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrMissing
	}
	return nil
}

func (s *Store) Comments(ctx context.Context, viewer, id int64) ([]Comment, error) {
	return s.comments(ctx, viewer, id)
}

// Metadata processes one durable job. Failed requests retry three times; saving
// links never depends on this worker or the provider being reachable.
func (s *Store) Metadata(ctx context.Context, fetch func(context.Context, string) (media.Metadata, error)) error {
	var id int64
	var video string
	var attempts int
	err := s.db.QueryRowContext(ctx, `SELECT m.id,m.video_id,j.attempts FROM metadata_jobs j JOIN media m ON m.id=j.media_id
 WHERE j.attempts<3 AND j.next_attempt<=? ORDER BY m.id LIMIT 1`, time.Now().Unix()).Scan(&id, &video, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	meta, fetchErr := fetch(ctx, video)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if fetchErr != nil {
		_, err = s.db.ExecContext(ctx, "UPDATE metadata_jobs SET attempts=attempts+1,next_attempt=? WHERE media_id=?", time.Now().Add(time.Duration(attempts+1)*time.Minute).Unix(), id)
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE media SET title=CASE WHEN title=original_url THEN ? ELSE title END,artist=CASE WHEN artist='' THEN ? ELSE artist END,thumbnail=? WHERE id=?", meta.Title, meta.Artist, meta.Thumbnail, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM metadata_jobs WHERE media_id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// Backup uses SQLite's consistent snapshot mechanism, including WAL contents.
// SQLite refuses to overwrite an existing target file.
func (s *Store) Backup(ctx context.Context, target string) error {
	_, err := s.db.ExecContext(ctx, "VACUUM INTO ?", target)
	return err
}

func (s *Store) Healthy(ctx context.Context) error { return s.db.PingContext(ctx) }
