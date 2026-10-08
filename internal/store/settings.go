// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/airencracken/comfylib/reference"
	"net/url"
	"strings"
)

var ErrForbidden = errors.New("permission denied")

// Settings are instance-wide presentation and joining policy. They confer no
// access to recommendations, groups or personal listening notes.
type Settings struct {
	Name, WelcomeTitle, WelcomeText, HouseRules, OwnerContact, SourceURL string
	BaseURL, WitmootURL, JoinMode                                        string
	ShowVersion                                                          bool
	Mascot, Favicon                                                      bool
}

func DefaultSettings(base, witmoot string) Settings {
	return Settings{Name: "Songstead", WelcomeTitle: "Keep the good songs close.", WelcomeText: "Save a recommendation, listen when you have a moment, and let your friend know what you thought.", SourceURL: "https://github.com/airencracken/songstead", BaseURL: base, WitmootURL: witmoot, JoinMode: "invite"}
}

type settingsReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readSettings(ctx context.Context, db settingsReader, defaults Settings) (Settings, error) {
	var raw string
	err := db.QueryRowContext(ctx, "SELECT content FROM instance_settings WHERE id=1").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return defaults, nil
	}
	if err != nil {
		return Settings{}, err
	}
	var settings Settings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return Settings{}, err
	}
	return settings, nil
}
func (s *Store) Settings(ctx context.Context, defaults Settings) (Settings, error) {
	settings, err := readSettings(ctx, s.db, defaults)
	if err != nil {
		return Settings{}, err
	}
	err = s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM branding_assets WHERE name='mascot'), EXISTS(SELECT 1 FROM branding_assets WHERE name='favicon')").Scan(&settings.Mascot, &settings.Favicon)
	return settings, err
}

func ValidateSettings(v Settings) error {
	if !validText(v.Name, 1, 80) || strings.ContainsAny(v.Name, "\r\n\t") || !validText(v.WelcomeTitle, 1, 120) || !validText(v.WelcomeText, 0, 2000) || !validText(v.HouseRules, 0, 5000) || !validText(v.OwnerContact, 0, 500) {
		return ErrInvalid
	}
	if v.JoinMode != "closed" && v.JoinMode != "invite" && v.JoinMode != "open" {
		return ErrInvalid
	}
	for _, raw := range []string{v.BaseURL, v.WitmootURL, v.SourceURL} {
		if raw == "" {
			continue
		}
		if len(raw) > 512 {
			return ErrInvalid
		}
		u, err := url.Parse(raw)
		if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return ErrInvalid
		}
		if _, err := reference.Handoff(raw, reference.Draft{Source: "https://source.invalid"}); err != nil {
			return ErrInvalid
		}
	}
	// Songstead routes live at the origin root; Witmoot may use a path prefix.
	if v.BaseURL != "" {
		u, _ := url.Parse(v.BaseURL)
		if u.Path != "" && u.Path != "/" {
			return ErrInvalid
		}
	}
	return nil
}
func requireOwner(ctx context.Context, tx *sql.Tx, actor int64) error {
	var ok bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id=? AND role='owner' AND suspended=0)", actor).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}
func (s *Store) SaveSettings(ctx context.Context, actor int64, v *Settings) error {
	if v != nil {
		if err := ValidateSettings(*v); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireOwner(ctx, tx, actor); err != nil {
		return err
	}
	if v == nil {
		_, err = tx.ExecContext(ctx, "DELETE FROM instance_settings")
	} else {
		raw, encodeErr := json.Marshal(v)
		if encodeErr != nil {
			return encodeErr
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO instance_settings(id,content) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET content=excluded.content", string(raw))
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
