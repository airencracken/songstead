// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"github.com/airencracken/comfylib/brandimage"
)

func (s *Store) SaveBrandingAssets(ctx context.Context, actor int64, mascot, favicon []byte, removeMascot, removeFavicon bool) error {
	var err error
	if len(mascot) > 0 {
		mascot, err = brandimage.Normalize(mascot)
		if err != nil {
			return ErrInvalid
		}
	}
	if len(favicon) > 0 {
		favicon, err = brandimage.Normalize(favicon)
		if err != nil {
			return ErrInvalid
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
	for _, v := range []struct {
		name   string
		data   []byte
		remove bool
	}{{"mascot", mascot, removeMascot}, {"favicon", favicon, removeFavicon}} {
		if v.remove {
			_, err = tx.ExecContext(ctx, "DELETE FROM branding_assets WHERE name=?", v.name)
		} else if len(v.data) > 0 {
			_, err = tx.ExecContext(ctx, "INSERT INTO branding_assets(name,content) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET content=excluded.content", v.name, v.data)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) BrandingAsset(ctx context.Context, name string) ([]byte, error) {
	if name != "mascot" && name != "favicon" {
		return nil, sql.ErrNoRows
	}
	var content []byte
	err := s.db.QueryRowContext(ctx, "SELECT content FROM branding_assets WHERE name=?", name).Scan(&content)
	return content, err
}
