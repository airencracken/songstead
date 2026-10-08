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
	ID       int64  `json:"id"`
	Username string `json:"username"`
}
type Recommendation struct {
	ID, MediaID, SenderID, RecipientID                                              int64
	Sender, Recipient, URL, Provider, Title, Artist, Thumbnail, Type, VideoID, Note string
	CreatedAt                                                                       int64
	Listening                                                                       string
	Rating                                                                          int
	PersonalNote                                                                    string
}
type Comment struct {
	ID, AuthorID, CreatedAt int64
	Author, Body            string
}
type Reaction struct {
	Listening string
	Rating    int
	Note      string
}

func Open(path string) (*Store, error) {
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
	if err := s.migrate(); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 1 {
		return fmt.Errorf("database schema %d is newer than this binary", version)
	}
	if version == 0 {
		data, err := migrations.ReadFile("migrations/001_initial.sql")
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(data)); err != nil {
			return err
		}
		if _, err := tx.Exec("PRAGMA user_version=1"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

var username = regexp.MustCompile(`^[A-Za-z0-9_-]{3,24}$`)

func ValidatePassword(password string) error {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 12 || len(password) > 72 {
		return errors.New("password needs at least 12 characters and at most 72 bytes")
	}
	return nil
}

func (s *Store) CreateUser(ctx context.Context, name, password string) (int64, error) {
	if !username.MatchString(name) {
		return 0, errors.New("username needs 3-24 letters, numbers, underscores or dashes")
	}
	if err := ValidatePassword(password); err != nil {
		return 0, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	result, err := s.db.ExecContext(ctx, "INSERT INTO users(username,password_hash,created_at) VALUES(?,?,?)", name, string(hash), time.Now().Unix())
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
	return tx.Commit()
}

func (s *Store) Credentials(ctx context.Context, name string) (User, string, error) {
	var u User
	var hash string
	err := s.db.QueryRowContext(ctx, "SELECT id,username,password_hash FROM users WHERE username=?", name).Scan(&u.ID, &u.Username, &hash)
	return u, hash, err
}

func (s *Store) Users(ctx context.Context, except int64) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,username FROM users WHERE id!=? ORDER BY username", except)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) Session(ctx context.Context, secret string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, "SELECT u.id,u.username FROM users u JOIN sessions s ON s.user_id=u.id WHERE s.token_hash=? AND s.expires_at>?", token.Hash(secret), time.Now().Unix()).Scan(&u.ID, &u.Username)
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
	result, err := tx.ExecContext(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) SELECT ?,id,? FROM users WHERE id=? AND password_hash=?", hash, time.Now().Add(7*24*time.Hour).Unix(), user, verifiedHash)
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
	link, err := media.Parse(raw)
	if err != nil {
		return 0, err
	}
	if sender == recipient || !validText(note, 0, 2000) {
		return 0, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "INSERT INTO media(original_url,provider,title,media_type,video_id) VALUES(?,?,?,?,?)", link.Original, link.Provider, link.Title, link.Type, link.VideoID)
	if err != nil {
		return 0, err
	}
	mid, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	result, err = tx.ExecContext(ctx, "INSERT INTO recommendations(media_id,sender_id,note,created_at) VALUES(?,?,?,?)", mid, sender, note, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO recommendation_destinations(recommendation_id,user_id) VALUES(?,?)", id, recipient); err != nil {
		return 0, err
	}
	if link.VideoID != "" {
		if _, err := tx.ExecContext(ctx, "INSERT INTO metadata_jobs(media_id) VALUES(?)", mid); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

const selectRecommendation = `SELECT r.id,m.id,r.sender_id,d.user_id,sender.username,recipient.username,
 m.original_url,m.provider,m.title,m.artist,m.thumbnail,m.media_type,m.video_id,r.note,r.created_at,
 coalesce(x.listening,'unheard'),coalesce(x.rating,0),coalesce(x.note,'')
 FROM recommendations r JOIN media m ON m.id=r.media_id
 JOIN recommendation_destinations d ON d.recommendation_id=r.id
 JOIN users sender ON sender.id=r.sender_id JOIN users recipient ON recipient.id=d.user_id
 LEFT JOIN reactions x ON x.recommendation_id=r.id AND x.user_id=? `

// The same visibility predicate protects details, comments and mutations.
const visible = `(r.sender_id=? OR d.user_id=?)`

type scanner interface{ Scan(...any) error }

func scanRecommendation(row scanner) (Recommendation, error) {
	var r Recommendation
	err := row.Scan(&r.ID, &r.MediaID, &r.SenderID, &r.RecipientID, &r.Sender, &r.Recipient, &r.URL, &r.Provider, &r.Title, &r.Artist, &r.Thumbnail, &r.Type, &r.VideoID, &r.Note, &r.CreatedAt, &r.Listening, &r.Rating, &r.PersonalNote)
	return r, err
}

func (s *Store) Recommendation(ctx context.Context, viewer, id int64) (Recommendation, error) {
	r, err := scanRecommendation(s.db.QueryRowContext(ctx, selectRecommendation+" WHERE r.id=? AND "+visible, viewer, id, viewer, viewer))
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrMissing
	}
	return r, err
}

func (s *Store) List(ctx context.Context, viewer int64, history bool, status string, limit, offset int) ([]Recommendation, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, ErrInvalid
	}
	if status != "" && !validListening(status) {
		return nil, ErrInvalid
	}
	where := "d.user_id=?"
	args := []any{viewer, viewer}
	if history {
		where = visible
		args = append(args, viewer)
	}
	if status != "" {
		where += " AND coalesce(x.listening,'unheard')=?"
		args = append(args, status)
	}
	order := "r.created_at DESC,r.id DESC"
	if !history {
		order = "(coalesce(x.listening,'unheard')='unheard') DESC," + order
	}
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, selectRecommendation+" WHERE "+where+" ORDER BY "+order+" LIMIT ? OFFSET ?", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Recommendation{}
	for rows.Next() {
		r, err := scanRecommendation(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func validListening(s string) bool {
	return s == "unheard" || s == "listened" || s == "revisit" || s == "dismissed"
}

func (s *Store) React(ctx context.Context, viewer, id int64, reaction Reaction) error {
	if !validListening(reaction.Listening) || reaction.Rating < -1 || reaction.Rating > 1 || !validText(reaction.Note, 0, 2000) {
		return ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO reactions(recommendation_id,user_id,listening,rating,note,updated_at)
 SELECT r.id,?,?,?,?,? FROM recommendations r JOIN recommendation_destinations d ON d.recommendation_id=r.id
 WHERE r.id=? AND `+visible+` ON CONFLICT(recommendation_id,user_id) DO UPDATE SET
 listening=excluded.listening,rating=excluded.rating,note=excluded.note,updated_at=excluded.updated_at`, viewer, reaction.Listening, reaction.Rating, reaction.Note, time.Now().Unix(), id, viewer, viewer)
	return mutationResult(result, err)
}

func (s *Store) AddComment(ctx context.Context, viewer, id int64, body string) error {
	if !validText(body, 1, 2000) || strings.TrimSpace(body) == "" {
		return ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO comments(recommendation_id,author_id,body,created_at)
 SELECT r.id,?,?,? FROM recommendations r JOIN recommendation_destinations d ON d.recommendation_id=r.id
 WHERE r.id=? AND `+visible, viewer, body, time.Now().Unix(), id, viewer, viewer)
	return mutationResult(result, err)
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
	// Even an empty list must distinguish an inaccessible recommendation.
	if _, err := s.Recommendation(ctx, viewer, id); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.author_id,c.created_at,u.username,c.body
 FROM comments c JOIN users u ON u.id=c.author_id JOIN recommendations r ON r.id=c.recommendation_id
 JOIN recommendation_destinations d ON d.recommendation_id=r.id WHERE r.id=? AND `+visible+` ORDER BY c.id`, id, viewer, viewer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.AuthorID, &c.CreatedAt, &c.Author, &c.Body); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
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
	if _, err := tx.ExecContext(ctx, "UPDATE media SET title=?,artist=?,thumbnail=? WHERE id=?", meta.Title, meta.Artist, meta.Thumbnail, id); err != nil {
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
