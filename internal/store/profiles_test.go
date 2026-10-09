// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"bytes"
	"context"
	"errors"
	"github.com/airencracken/songstead/internal/media"
	"image"
	"image/color"
	"image/gif"
	"path/filepath"
	"reflect"
	"testing"
)

func animatedPicture(t *testing.T) []byte {
	t.Helper()
	v := &gif.GIF{Delay: []int{10, 10}}
	for i := 0; i < 2; i++ {
		img := image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White})
		img.SetColorIndex(i, i, 1)
		v.Image = append(v.Image, img)
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, v); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestProfilesAtomicityMotionPrivacyAndSchema(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	if err := s.SetPicture(ctx, u[0], animatedPicture(t)); err != nil {
		t.Fatal(err)
	}
	p, err := s.Profile(ctx, u[0])
	if err != nil || len(p.Still) == 0 || len(p.Animation) == 0 || !p.Animate {
		t.Fatal(p, err)
	}
	if err = s.SetPicture(ctx, u[0], []byte("<svg/>")); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	after, _ := s.Profile(ctx, u[0])
	if !reflect.DeepEqual(p, after) {
		t.Fatal("invalid upload changed profile")
	}
	data, kind, err := s.Picture(ctx, u[1], u[0], false)
	if err != nil || kind != "image/gif" || !bytes.Equal(data, p.Animation) {
		t.Fatal("animation missing", kind, err)
	}
	if err = s.SetAnimationPreference(ctx, u[1], false); err != nil {
		t.Fatal(err)
	}
	for _, still := range []bool{false, true} {
		data, kind, err = s.Picture(ctx, u[1], u[0], still)
		if err != nil || kind != "image/png" || !bytes.Equal(data, p.Still) {
			t.Fatal("disabled motion still animated", kind, err)
		}
	}
	data, kind, err = s.Picture(ctx, u[0], u[0], true)
	if err != nil || kind != "image/png" || !bytes.Equal(data, p.Still) {
		t.Fatal("reduced motion", kind, err)
	}
	user, _, _ := s.Credentials(ctx, "bobby")
	archive, err := s.Export(ctx, user)
	if err != nil || archive.Profile.Animate || len(archive.Profile.Animation) > 0 {
		t.Fatal("export leaked another profile", archive.Profile, err)
	}
	for _, sql := range []string{"UPDATE user_profiles SET animate=2", "UPDATE user_profiles SET still=X'00'", "UPDATE user_profiles SET animation=X'00'"} {
		if _, err = s.db.Exec(sql); err == nil {
			t.Fatal("invalid schema data accepted", sql)
		}
	}
	backup := filepath.Join(t.TempDir(), "snapshot.db")
	if err = s.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if err = ValidateSnapshot(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if err = s.SetPicture(ctx, u[0], nil); err != nil {
		t.Fatal(err)
	}
	p, _ = s.Profile(ctx, u[0])
	if len(p.Still) > 0 || len(p.Animation) > 0 || !p.Animate {
		t.Fatal("remove changed preferences", p)
	}
}

func TestLabelChoicesAndPreviewAudience(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	_, err := s.ShareLabeledMusic(ctx, u[0], "https://music.example/a", "", "track", "Visible", "", Labels{"Jazz", []string{"Acoustic"}})
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.RecommendLabeledMusic(ctx, u[0], u[1], 0, "https://youtu.be/dQw4w9WgXcQ", "", "track", "Private", "", Labels{"Secret", []string{"Hidden"}})
	if err != nil {
		t.Fatal(err)
	}
	genres, tags, err := s.LabelChoices(ctx, u[0], true)
	if err != nil || !reflect.DeepEqual(genres, []string{"Jazz"}) || !reflect.DeepEqual(tags, []string{"Acoustic"}) {
		t.Fatal(genres, tags, err)
	}
	genres, _, err = s.LabelChoices(ctx, u[2], false)
	if err != nil || !reflect.DeepEqual(genres, []string{"Jazz"}) {
		t.Fatal("private label leak", genres, err)
	}
	for _, viewer := range []int64{u[2], 9999} {
		if _, err = s.PreviewPending(ctx, viewer, id); !errors.Is(err, ErrMissing) {
			t.Fatal("preview leak", err)
		}
		if err = s.RetryPreview(ctx, viewer, id); !errors.Is(err, ErrMissing) {
			t.Fatal("unauthorized retry", err)
		}
	}
	if _, err = s.db.Exec("UPDATE metadata_jobs SET attempts=3"); err != nil {
		t.Fatal(err)
	}
	pending, _ := s.PreviewPending(ctx, u[1], id)
	if pending {
		t.Fatal("exhausted retry pending")
	}
	if err = s.RetryPreview(ctx, u[1], id); err != nil {
		t.Fatal(err)
	}
	pending, _ = s.PreviewPending(ctx, u[1], id)
	if !pending {
		t.Fatal("retry not queued")
	}
}

func TestSchemaSixUpgradeRetriesMissingArtwork(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	first, err := s.Recommend(ctx, u[0], u[1], "https://youtu.be/dQw4w9WgXcQ", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Recommend(ctx, u[0], u[1], "https://youtu.be/aaaaaaaaaaa", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Metadata(ctx, func(context.Context, string) (media.Metadata, error) {
		return media.Metadata{Title: "Cached title", Artwork: artworkFixture(t)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("UPDATE metadata_jobs SET attempts=3; DROP TABLE user_profiles; PRAGMA user_version=6"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "old.db")
	if err = s.Backup(ctx, path); err != nil {
		t.Fatal(err)
	}
	if err = ValidateSnapshot(ctx, path); err != nil {
		t.Fatal("schema 6 backup", err)
	}
	if old, err := OpenCurrent(path); err == nil {
		old.Close()
		t.Fatal("CLI migrated old schema")
	}
	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if _, err = upgraded.Artwork(ctx, u[1], first); err != nil {
		t.Fatal("cached artwork lost", err)
	}
	pending, err := upgraded.PreviewPending(ctx, u[1], first)
	if err != nil || pending {
		t.Fatal("cached artwork requeued", pending, err)
	}
	pending, err = upgraded.PreviewPending(ctx, u[1], second)
	if err != nil || !pending {
		t.Fatal("missing artwork not retried", pending, err)
	}
	p, err := upgraded.Profile(ctx, u[1])
	if err != nil || !p.Animate || len(p.Still) > 0 {
		t.Fatal("migration profile defaults", p, err)
	}
}
