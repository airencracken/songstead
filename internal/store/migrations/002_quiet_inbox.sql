ALTER TABLE users ADD COLUMN annotation_mode TEXT NOT NULL DEFAULT 'immediate' CHECK(annotation_mode IN ('immediate','spoiler-free','hidden'));
CREATE TABLE groups (id INTEGER PRIMARY KEY, owner_id INTEGER NOT NULL REFERENCES users(id), name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 80));
CREATE TABLE group_members (group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE, user_id INTEGER NOT NULL REFERENCES users(id), PRIMARY KEY(group_id,user_id));
ALTER TABLE recommendations ADD COLUMN group_id INTEGER REFERENCES groups(id);
ALTER TABLE recommendations ADD COLUMN source_url TEXT NOT NULL DEFAULT '';
UPDATE recommendations SET source_url=(SELECT original_url FROM media WHERE id=media_id);
ALTER TABLE media ADD COLUMN canonical_key TEXT NOT NULL DEFAULT '';
ALTER TABLE media ADD COLUMN kind TEXT NOT NULL DEFAULT 'link' CHECK(kind IN ('link','track','album','artist'));
UPDATE media SET kind=CASE WHEN media_type IN ('song','video') THEN 'track' WHEN media_type='album' THEN 'album' ELSE 'link' END;
CREATE TABLE music_states (media_id INTEGER NOT NULL REFERENCES media(id), user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, listening TEXT NOT NULL CHECK(listening IN ('unheard','saved','explored','listened','revisit','dismissed')), updated_at INTEGER NOT NULL, PRIMARY KEY(media_id,user_id));
CREATE TABLE recordings (id INTEGER PRIMARY KEY, canonical_key TEXT NOT NULL UNIQUE, url TEXT NOT NULL, title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 160), duration INTEGER NOT NULL DEFAULT 0 CHECK(duration BETWEEN 0 AND 86400));
CREATE TABLE media_recordings (media_id INTEGER NOT NULL REFERENCES media(id), recording_id INTEGER NOT NULL REFERENCES recordings(id), PRIMARY KEY(media_id,recording_id));
CREATE TABLE annotation_offsets (comment_id INTEGER NOT NULL REFERENCES comments(id) ON DELETE CASCADE, recording_id INTEGER NOT NULL REFERENCES recordings(id), ordinal INTEGER NOT NULL, seconds INTEGER NOT NULL CHECK(seconds BETWEEN 0 AND 86400), start_byte INTEGER NOT NULL, end_byte INTEGER NOT NULL CHECK(end_byte>start_byte), PRIMARY KEY(comment_id,ordinal));
CREATE TABLE listening_positions (user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, recording_id INTEGER NOT NULL REFERENCES recordings(id), seconds INTEGER NOT NULL CHECK(seconds BETWEEN 0 AND 86400), PRIMARY KEY(user_id,recording_id));
CREATE TABLE discussions (media_id INTEGER NOT NULL REFERENCES media(id), user_id INTEGER NOT NULL REFERENCES users(id), url TEXT NOT NULL, PRIMARY KEY(media_id,user_id,url));
CREATE TRIGGER annotation_recording_guard BEFORE INSERT ON annotation_offsets BEGIN
 SELECT RAISE(ABORT,'annotation recording does not belong to this music') WHERE NOT EXISTS (
  SELECT 1 FROM comments c JOIN recommendations r ON r.id=c.recommendation_id
  JOIN media_recordings mr ON mr.media_id=r.media_id
  WHERE c.id=NEW.comment_id AND mr.recording_id=NEW.recording_id
 );
 SELECT RAISE(ABORT,'invalid annotation span') WHERE NEW.ordinal<0 OR NEW.start_byte<0 OR NEW.end_byte>(SELECT length(CAST(body AS BLOB)) FROM comments WHERE id=NEW.comment_id);
 SELECT RAISE(ABORT,'annotation beyond duration') WHERE EXISTS(SELECT 1 FROM recordings WHERE id=NEW.recording_id AND duration>0 AND NEW.seconds>duration);
END;
