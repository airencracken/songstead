// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGroupPermissionsProvenanceAndRemoval(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	g, err := s.SaveGroup(ctx, u[0], 0, "Music room", []int64{u[1]})
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.RecommendMusic(ctx, u[0], 0, g, "https://music.example/album", "For the room", "album", "Our album", "Artist")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Recommendation(ctx, u[1], id)
	if err != nil || r.Group != "Music room" || r.Note != "For the room" {
		t.Fatal(r, err)
	}
	if err = s.AddComment(ctx, u[1], id, "a thought"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Recommendation(ctx, u[2], id); !errors.Is(err, ErrMissing) {
		t.Fatal("outsider", err)
	}
	if _, err = s.SaveGroup(ctx, u[1], g, "Hijack", []int64{u[2]}); !errors.Is(err, ErrMissing) {
		t.Fatal("nonowner changed group", err)
	}
	if _, err = s.SaveGroup(ctx, u[0], g, "Music room", nil); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Comments(ctx, u[1], id); !errors.Is(err, ErrMissing) {
		t.Fatal("removed member access", err)
	}
	if err = s.React(ctx, u[1], id, Reaction{"saved", 0, ""}); !errors.Is(err, ErrMissing) {
		t.Fatal("removed member mutation", err)
	}
}
func TestDuplicateMusicKeepsContextAndPrivateState(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	one := recommend(t, s, u[0], u[1])
	two, err := s.Recommend(ctx, u[2], u[1], "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "different friend note")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.React(ctx, u[1], one, Reaction{"saved", 1, "mine"}); err != nil {
		t.Fatal(err)
	}
	r, err := s.Recommendation(ctx, u[1], two)
	if err != nil || r.Listening != "saved" || r.Sender != "carol" || r.Note != "different friend note" || r.PersonalNote != "" {
		t.Fatal(r, err)
	}
	sender, _ := s.Recommendation(ctx, u[2], two)
	if sender.Listening != "unheard" {
		t.Fatal("private state leaked")
	}
	rows, err := s.Browse(ctx, u[1], false, Filter{Person: u[2]}, 10, 0)
	if err != nil || len(rows) != 1 || rows[0].ID != two {
		t.Fatal(rows, err)
	}
	if err = s.AddComment(ctx, u[0], two, "wrong context"); !errors.Is(err, ErrMissing) {
		t.Fatal("same music granted access", err)
	}
}
func TestAnnotationsAtomicityAndRecordingSelection(t *testing.T) {
	s, u := fixture(t)
	ctx := t.Context()
	id := recommend(t, s, u[0], u[1])
	tracks, err := s.Recordings(ctx, u[1], id)
	if err != nil || len(tracks) != 1 {
		t.Fatal(tracks, err)
	}
	if err = s.AddRecording(ctx, u[1], id, tracks[0].URL, "Track", 200); err != nil {
		t.Fatal(err)
	}
	if err = s.Annotate(ctx, u[1], id, 0, "First 0:00, bass 2:43, outside 3:21, bad 1:99"); err != nil {
		t.Fatal(err)
	}
	comments, err := s.Comments(ctx, u[0], id)
	if err != nil || len(comments) != 1 || len(comments[0].Offsets) != 2 || comments[0].Offsets[1].Seconds != 163 {
		t.Fatal(comments, err)
	}
	if err = s.AddRecording(ctx, u[1], id, tracks[0].URL, "Track", 100); !errors.Is(err, ErrInvalid) {
		t.Fatal("duration invalidated existing markers", err)
	}
	album, err := s.RecommendMusic(ctx, u[0], u[1], 0, "https://music.example/album", "", "album", "Album", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Annotate(ctx, u[1], album, 0, "2:43"); !errors.Is(err, ErrInvalid) {
		t.Fatal("ambiguous album time", err)
	}
	if err = s.Annotate(ctx, u[1], album, tracks[0].ID, "2:43"); !errors.Is(err, ErrInvalid) {
		t.Fatal("foreign recording", err)
	}
	if err = s.AddRecording(ctx, u[1], album, tracks[0].URL, "Track", 200); err != nil {
		t.Fatal(err)
	}
	if err = s.Annotate(ctx, u[1], album, tracks[0].ID, "2:43"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("CREATE TRIGGER fail_offsets BEFORE INSERT ON annotation_offsets BEGIN SELECT RAISE(ABORT,'fail'); END"); err != nil {
		t.Fatal(err)
	}
	if err = s.Annotate(ctx, u[1], id, 0, "rollback at 1:00"); err == nil {
		t.Fatal("accepted failure")
	}
	comments, _ = s.Comments(ctx, u[1], id)
	if len(comments) != 1 {
		t.Fatal("orphan comment")
	}
}
func TestSpoilerVisibilityMultipleOffsets(t *testing.T) {
	c := []Comment{{ID: 1, Body: "ordinary"}, {ID: 2, Body: "spoiler", Offsets: []Offset{{RecordingID: 4, Seconds: 1}, {RecordingID: 4, Seconds: 30}}}}
	tracks := []Recording{{ID: 4, Position: 10}}
	if got := VisibleComments(c, tracks, "spoiler-free", false); len(got) != 1 {
		t.Fatal("later spoiler leaked", got)
	}
	tracks[0].Position = 30
	if got := VisibleComments(c, tracks, "spoiler-free", false); len(got) != 2 {
		t.Fatal(got)
	}
	if got := VisibleComments(c, tracks, "hidden", false); len(got) != 1 {
		t.Fatal(got)
	}
	if got := VisibleComments(c, tracks, "hidden", true); len(got) != 2 {
		t.Fatal(got)
	}
	if got := VisibleComments(c, nil, "spoiler-free", false); len(got) != 1 {
		t.Fatal("missing progress revealed", got)
	}
}
func TestReactionAtomicity(t *testing.T) {
	s, u := fixture(t)
	id := recommend(t, s, u[0], u[1])
	s.db.Exec("CREATE TRIGGER fail_state BEFORE INSERT ON music_states BEGIN SELECT RAISE(ABORT,'fail'); END")
	if err := s.React(t.Context(), u[1], id, Reaction{"saved", 1, "private"}); err == nil {
		t.Fatal("accepted")
	}
	var n int
	s.db.QueryRow("SELECT count(*) FROM reactions").Scan(&n)
	if n != 0 {
		t.Fatal("partial reaction")
	}
}
func TestV1MigrationMergesMusicWithoutLosingGifts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("migrations/001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO users VALUES(1,'alice','hash',1),(2,'bobby','hash',1); INSERT INTO media(id,original_url,provider,title,media_type,video_id) VALUES(1,'https://youtu.be/dQw4w9WgXcQ','YouTube','Title','song','dQw4w9WgXcQ'),(2,'https://www.youtube.com/watch?v=dQw4w9WgXcQ','YouTube','Title','song','dQw4w9WgXcQ'); INSERT INTO recommendations VALUES(1,1,1,'first',1),(2,2,1,'second',2);INSERT INTO recommendation_destinations VALUES(1,2),(2,2);INSERT INTO reactions VALUES(1,2,'listened',1,'note',1),(2,2,'revisit',0,'other',2);INSERT INTO comments VALUES(1,1,2,'bass at 2:43',1);PRAGMA user_version=1;`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, _ := s.Recommendation(t.Context(), 2, 1)
	b, _ := s.Recommendation(t.Context(), 2, 2)
	if a.MediaID != b.MediaID || a.Note != "first" || b.Note != "second" || a.PersonalNote != "note" || a.Listening != "revisit" {
		t.Fatal(a, b)
	}
	comments, err := s.Comments(t.Context(), 2, 1)
	if err != nil || len(comments) != 1 || len(comments[0].Offsets) != 1 {
		t.Fatal("legacy track annotations lost", comments, err)
	}
	if err = ValidateSnapshot(t.Context(), path); err != nil {
		t.Fatal(err)
	}
}

func TestAccountExportRetainsPrivateAnnotationSettingsAndReferences(t *testing.T) {
	s, u := fixture(t)
	id := recommend(t, s, u[0], u[1])
	s.AddComment(t.Context(), u[1], id, "at 2:43")
	tracks, _ := s.Recordings(t.Context(), u[1], id)
	s.SetPosition(t.Context(), u[1], id, tracks[0].ID, 200)
	s.SetPosition(t.Context(), u[0], id, tracks[0].ID, 400)
	s.SetAnnotationMode(t.Context(), u[1], "hidden")
	s.LinkDiscussion(t.Context(), u[1], id, "https://board.example/topics/2")
	a, err := s.Export(t.Context(), User{ID: u[1], Username: "bobby"})
	if err != nil || a.Version != 2 || a.AnnotationMode != "hidden" || len(a.Comments) != 1 || len(a.Comments[0].Offsets) != 1 || len(a.Recordings) != 1 || a.Recordings[0].Position != 200 || len(a.Discussions) != 1 {
		t.Fatal(a, err)
	}
}

func TestQuietSchemaConstraints(t *testing.T) {
	s, u := fixture(t)
	id := recommend(t, s, u[0], u[1])
	s.AddComment(t.Context(), u[1], id, "at 1:00")
	tracks, _ := s.Recordings(t.Context(), u[1], id)
	var comment int64
	s.db.QueryRow("SELECT id FROM comments LIMIT 1").Scan(&comment)
	s.AddRecording(t.Context(), u[1], id, tracks[0].URL, "Track", 100)
	for _, args := range [][]any{{comment, tracks[0].ID, 9, 60, -1, 4}, {comment, tracks[0].ID, 9, 60, 0, 9000}, {comment, tracks[0].ID, -1, 60, 0, 4}, {comment, tracks[0].ID, 9, 101, 0, 4}, {comment, 9999, 9, 10, 0, 4}} {
		if _, err := s.db.Exec("INSERT INTO annotation_offsets VALUES(?,?,?,?,?,?)", args...); err == nil {
			t.Fatal("invalid schema offset accepted", args)
		}
	}
	if _, err := s.db.Exec("INSERT INTO music_states VALUES(?,?,'overdue',0)", id, u[1]); err == nil {
		t.Fatal("obligation state accepted")
	}
	g, err := s.SaveGroup(t.Context(), u[0], 0, "Room", []int64{u[1]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveGroup(t.Context(), u[0], g, "Changed", []int64{999999}); err == nil {
		t.Fatal("invalid member accepted")
	}
	members, _ := s.GroupMembers(t.Context(), u[1], g)
	if len(members) != 2 {
		t.Fatal("failed update changed membership", members)
	}
}

func TestMusicPaginationRetainsAllDuplicateContexts(t *testing.T) {
	s, u := fixture(t)
	for i := 0; i < 55; i++ {
		recommend(t, s, u[0], u[1])
	}
	rows, err := s.Browse(t.Context(), u[1], false, Filter{GroupByMusic: true}, 1, 0)
	if err != nil || len(rows) != 55 {
		t.Fatal("lost provenance", len(rows), err)
	}
	rows, err = s.Browse(t.Context(), u[1], false, Filter{GroupByMusic: true}, 1, 1)
	if err != nil || len(rows) != 0 {
		t.Fatal("duplicate listening on next page", len(rows), err)
	}
}
