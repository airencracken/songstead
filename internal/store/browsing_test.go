// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"github.com/airencracken/songstead/internal/media"
)

func artworkFixture(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestLabelsAudienceIsolationAndSenderPermissions(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	private, err := s.RecommendLabeledMusic(ctx, u[0], u[1], 0, "https://music.example/same", "private note", "track", "A song", "Artist", Labels{"Private genre", []string{"Secret tag"}})
	if err != nil {
		t.Fatal(err)
	}
	shared, err := s.ShareLabeledMusic(ctx, u[0], "https://music.example/same", "shared note", "track", "A song", "Artist", Labels{"Jazz", []string{"Live", "live", " café "}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Recommendation(ctx, u[2], shared)
	if err != nil || r.Genre != "Jazz" || !reflect.DeepEqual(r.Tags, []string{"Live", "café"}) {
		t.Fatal(r, err)
	}
	for _, actor := range []int64{u[1], u[2], 0, 99999} {
		if err := s.SetLabels(ctx, actor, shared, Labels{"Changed", nil}); !errors.Is(err, ErrMissing) {
			t.Fatal("nonsender changed labels", actor, err)
		}
	}
	if err := s.SetLabels(ctx, u[0], shared, Labels{"Ambient", []string{"Instrumental"}}); err != nil {
		t.Fatal(err)
	}
	for _, viewer := range []int64{u[0], u[1]} {
		r, err := s.Recommendation(ctx, viewer, private)
		if err != nil || r.Genre != "Private genre" || strings.Join(r.Tags, ",") != "Secret tag" {
			t.Fatal("canonical duplicate leaked labels", r, err)
		}
	}
	if _, err := s.Recommendation(ctx, u[2], private); !errors.Is(err, ErrMissing) {
		t.Fatal(err)
	}
	for _, viewer := range u {
		rows, err := s.Browse(ctx, viewer, false, Filter{Recent: true}, 100, 0)
		if err != nil || len(rows) != 1 || rows[0].ID != shared || rows[0].Genre != "Ambient" {
			t.Fatal(rows, err)
		}
	}
	// An owner still has no permission to read or relabel another private send.
	owner, err := s.CreateOwner(ctx, "owner", "a-long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetLabels(ctx, owner, private, Labels{"Changed", nil}); !errors.Is(err, ErrMissing) {
		t.Fatal("owner leaked private classification", err)
	}
	archive, err := s.Export(ctx, User{ID: u[2], Username: "carol"})
	data, _ := json.Marshal(archive)
	if err != nil || bytes.Contains(data, []byte("Private genre")) || bytes.Contains(data, []byte("Secret tag")) {
		t.Fatal("export leaked private labels", string(data), err)
	}
}

func TestDiscoveryPreferencesOrderingPaginationAndPrivacy(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	var ids []int64
	for i, labels := range []Labels{{"Jazz", []string{"Live"}}, {"Metal", []string{"Live"}}, {"Folk", []string{"Acoustic"}}, {"", nil}, {"Électronique", []string{"café"}}} {
		id, err := s.ShareLabeledMusic(ctx, u[0], "https://music.example/"+string(rune('a'+i)), "", "track", "Song", "Artist", labels)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	prefs := DiscoveryPreferences{ExcludedGenres: []string{"metal"}, PreferredGenres: []string{"JAZZ", "électronique"}, PreferredTags: []string{"acoustic"}}
	if err := s.SetDiscoveryPreferences(ctx, u[1], prefs); err != nil {
		t.Fatal(err)
	}
	// Favorites lead, newest first within the same preference tier. Exclusion
	// wins even if the same recommendation also has a preferred tag.
	want := []int64{ids[4], ids[2], ids[0], ids[3]}
	for _, groupBy := range []bool{false, true} {
		var got []int64
		for offset := 0; offset < len(want); offset += 2 {
			rows, err := s.Browse(ctx, u[1], false, Filter{Recent: true, UsePreferences: true, GroupByMusic: groupBy}, 2, offset)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rows {
				got = append(got, r.ID)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("preference pagination changed order", groupBy, got, want)
		}
	}
	for _, tt := range []struct {
		viewer int64
		f      Filter
		count  int
	}{{u[1], Filter{Recent: true}, 5}, {u[2], Filter{Recent: true, UsePreferences: true}, 5}, {u[1], Filter{Recent: true, Genre: "ÉLECTRONIQUE"}, 1}, {u[1], Filter{Recent: true, Tag: "CAFÉ"}, 1}, {u[1], Filter{Recent: true, Genre: "%' OR 1=1 --"}, 0}} {
		rows, err := s.Browse(ctx, tt.viewer, false, tt.f, 100, 0)
		if err != nil || len(rows) != tt.count {
			t.Fatal(tt, rows, err)
		}
	}
	if err := s.SetDiscoveryPreferences(ctx, u[1], DiscoveryPreferences{ExcludedTags: []string{"LIVE"}, PreferredTags: []string{"live"}}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Browse(ctx, u[1], false, Filter{Recent: true, UsePreferences: true}, 100, 0)
	if err != nil || len(rows) != 3 {
		t.Fatal("tag exclusions lost", rows, err)
	}
	archive, err := s.Export(ctx, User{ID: u[1], Username: "bobby"})
	if err != nil || !reflect.DeepEqual(archive.DiscoveryPreferences.ExcludedTags, []string{"LIVE"}) {
		t.Fatal(archive, err)
	}
	other, err := s.Export(ctx, User{ID: u[2], Username: "carol"})
	if err != nil || len(other.DiscoveryPreferences.ExcludedTags) != 0 {
		t.Fatal("preferences leaked", other, err)
	}
	if err := s.SetDiscoveryPreferences(ctx, u[1], DiscoveryPreferences{}); err != nil {
		t.Fatal(err)
	}
	rows, err = s.Browse(ctx, u[1], false, Filter{Recent: true, UsePreferences: true}, 100, 0)
	if err != nil || len(rows) != 5 || rows[0].ID != ids[4] {
		t.Fatal("clear did not restore newest first", rows, err)
	}
}

func TestLabelsValidationPropertiesSchemaAndAtomicity(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	id := recommend(t, s, u[0], u[1])
	for _, labels := range []Labels{{strings.Repeat("x", 81), nil}, {"bad\nline", nil}, {"Jazz, metal", nil}, {"", []string{strings.Repeat("x", 41)}}, {"", []string{"bad\x00"}}, {"", []string{"a,b"}}, {"", make([]string, 21)}} {
		if err := s.SetLabels(ctx, u[0], id, labels); !errors.Is(err, ErrInvalid) {
			t.Fatal("accepted invalid labels", labels, err)
		}
	}
	if err := quick.Check(func(raw string) bool {
		labels, err := ParseLabels(raw, 40)
		if err != nil {
			return true
		}
		again, err := ParseLabels(strings.Join(labels, ","), 40)
		return err == nil && reflect.DeepEqual(labels, again) && len(labels) <= 20
	}, &quick.Config{MaxCount: 500}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_labels BEFORE INSERT ON recommendation_labels BEGIN SELECT RAISE(ABORT,'injected failure'); END;`); err != nil {
		t.Fatal(err)
	}
	var before, after int
	s.db.QueryRow("SELECT count(*) FROM recommendations").Scan(&before)
	if _, err := s.ShareLabeledMusic(ctx, u[0], "https://music.example/atomic", "", "track", "Title", "Artist", Labels{"Jazz", nil}); err == nil {
		t.Fatal("injected failure accepted")
	}
	s.db.QueryRow("SELECT count(*) FROM recommendations").Scan(&after)
	if before != after {
		t.Fatal("failed labels left recommendation", before, after)
	}
	var mediaCount int
	s.db.QueryRow("SELECT count(*) FROM media WHERE original_url='https://music.example/atomic'").Scan(&mediaCount)
	if mediaCount != 0 {
		t.Fatal("failed share left music")
	}
	s.db.Exec("DROP TRIGGER reject_labels")
	if err := s.SetDiscoveryPreferences(ctx, u[1], DiscoveryPreferences{PreferredTags: []string{"Valid"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDiscoveryPreferences(ctx, u[1], DiscoveryPreferences{ExcludedGenres: []string{"Changed"}, PreferredTags: []string{"bad\nname"}}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	prefs, err := s.DiscoveryPreferences(ctx, u[1])
	if err != nil || len(prefs.ExcludedGenres) != 0 || !reflect.DeepEqual(prefs.PreferredTags, []string{"Valid"}) {
		t.Fatal("failed preference save partially applied", prefs, err)
	}
	for _, query := range []string{
		"INSERT INTO media_artwork VALUES(1,x'00')",
		"INSERT INTO recommendation_labels VALUES(1,'','','{}','[]')",
		"INSERT INTO recommendation_labels VALUES(1,'','','not-json','[]')",
		"INSERT INTO recommendation_labels VALUES(1,'" + strings.Repeat("x", 81) + "','','[]','[]')",
		"UPDATE discovery_preferences SET preferred_tags='{}'",
		"UPDATE discovery_preferences SET excluded_genres='bad-json'",
	} {
		if _, err := s.db.Exec(query); err == nil {
			t.Fatal("schema accepted invalid input", query)
		}
	}
}

func TestArtworkPermissionsRetryAndAtomicity(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	id := recommend(t, s, u[0], u[1])
	image := artworkFixture(t)
	meta := media.Metadata{Title: "A song", Artist: "Artist", Thumbnail: "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg"}
	if err := s.Metadata(ctx, func(context.Context, string) (media.Metadata, error) { return meta, nil }); err != nil {
		t.Fatal(err)
	}
	var attempts int
	s.db.QueryRow("SELECT attempts FROM metadata_jobs").Scan(&attempts)
	item, _ := s.Recommendation(ctx, u[1], id)
	if attempts != 1 || item.Title != "A song" || item.HasArtwork {
		t.Fatal("missing image lost text/retry", item, attempts)
	}
	s.db.Exec("UPDATE metadata_jobs SET next_attempt=0")
	if _, err := s.db.Exec(`CREATE TRIGGER reject_artwork BEFORE INSERT ON media_artwork BEGIN SELECT RAISE(ABORT,'injected'); END;`); err != nil {
		t.Fatal(err)
	}
	meta.Artwork = image
	if err := s.Metadata(ctx, func(context.Context, string) (media.Metadata, error) { return meta, nil }); err == nil {
		t.Fatal("injected cache failure accepted")
	}
	s.db.QueryRow("SELECT attempts FROM metadata_jobs").Scan(&attempts)
	if attempts != 1 {
		t.Fatal("cache failure removed job")
	}
	s.db.Exec("DROP TRIGGER reject_artwork")
	if err := s.Metadata(ctx, func(context.Context, string) (media.Metadata, error) { return meta, nil }); err != nil {
		t.Fatal(err)
	}
	for _, viewer := range []int64{u[0], u[1]} {
		data, err := s.Artwork(ctx, viewer, id)
		if err != nil || !bytes.Equal(data, image) {
			t.Fatal("cache mismatch", err)
		}
	}
	for _, viewer := range []int64{0, u[2], 99999} {
		if _, err := s.Artwork(ctx, viewer, id); !errors.Is(err, ErrMissing) {
			t.Fatal("private thumbnail exposed", viewer, err)
		}
	}
	if _, err := s.Artwork(ctx, u[1], 99999); !errors.Is(err, ErrMissing) {
		t.Fatal(err)
	}
	var jobs int
	s.db.QueryRow("SELECT count(*) FROM metadata_jobs").Scan(&jobs)
	if jobs != 0 {
		t.Fatal("cache success retained job")
	}
}

func TestBrowsingMigrationSnapshotAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "songstead.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateOwner(t.Context(), "owner", "a-long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.ShareMusic(t.Context(), u, "https://open.spotify.com/track/0sNOF9WDwhWunNAHPD3Baj", "", "track", "Manual title", "Manual artist")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAnnotationMode(t.Context(), u, "spoiler-free"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("DROP TABLE member_profiles; DROP TABLE user_profiles; DROP TABLE media_artwork; DROP TABLE recommendation_labels; DROP TABLE discovery_preferences; DELETE FROM metadata_jobs; PRAGMA user_version=5"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := ValidateSnapshot(t.Context(), path); err != nil {
		t.Fatal("schema 5 backup rejected", err)
	}
	if _, err := OpenCurrent(path); err == nil {
		t.Fatal("CLI migrated old schema")
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Metadata(t.Context(), func(_ context.Context, raw string) (media.Metadata, error) {
		if raw != "https://open.spotify.com/track/0sNOF9WDwhWunNAHPD3Baj" {
			t.Fatal("backfill fetched wrong link", raw)
		}
		return media.Metadata{Title: "Provider title", Artist: "Provider artist", Artwork: artworkFixture(t)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	item, err := s.Recommendation(t.Context(), u, id)
	if err != nil || item.Title != "Manual title" || item.Artist != "Manual artist" || !item.HasArtwork {
		t.Fatal("backfill changed manual metadata", item, err)
	}
	if err := s.SetLabels(t.Context(), u, id, Labels{"Jazz", []string{"Live"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDiscoveryPreferences(t.Context(), u, DiscoveryPreferences{PreferredGenres: []string{"Jazz"}}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := ValidateSnapshot(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	s, err = OpenCurrent(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	item, err = s.Recommendation(t.Context(), u, id)
	if err != nil || item.Genre != "Jazz" || !item.HasArtwork {
		t.Fatal("restart lost labels/cache", item, err)
	}
	mode, _ := s.AnnotationMode(t.Context(), u)
	prefs, err := s.DiscoveryPreferences(t.Context(), u)
	if err != nil || mode != "spoiler-free" || !reflect.DeepEqual(prefs.PreferredGenres, []string{"Jazz"}) {
		t.Fatal("restart lost preferences", mode, prefs, err)
	}
}
