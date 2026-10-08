-- Existing gifts keep their audience. Sharing with the instance is explicit.
ALTER TABLE recommendations ADD COLUMN visibility TEXT NOT NULL DEFAULT 'private'
 CHECK(visibility IN ('private','members'))
 CHECK(visibility='private' OR group_id IS NULL);
CREATE INDEX recommendations_recent ON recommendations(created_at DESC,id DESC)
 WHERE visibility='members';
