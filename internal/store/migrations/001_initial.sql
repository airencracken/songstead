CREATE TABLE users (
 id INTEGER PRIMARY KEY,
 username TEXT NOT NULL COLLATE NOCASE UNIQUE CHECK(length(username) BETWEEN 3 AND 24),
 password_hash TEXT NOT NULL,
 created_at INTEGER NOT NULL
);
CREATE TABLE sessions (
 token_hash TEXT PRIMARY KEY CHECK(length(token_hash) = 64),
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 expires_at INTEGER NOT NULL
);
CREATE TABLE media (
 id INTEGER PRIMARY KEY,
 original_url TEXT NOT NULL CHECK(length(original_url) BETWEEN 1 AND 4096),
 provider TEXT NOT NULL,
 title TEXT NOT NULL,
 artist TEXT NOT NULL DEFAULT '',
 thumbnail TEXT NOT NULL DEFAULT '',
 media_type TEXT NOT NULL DEFAULT 'url' CHECK(media_type IN ('url','song','album','video')),
 video_id TEXT NOT NULL DEFAULT ''
);
CREATE TABLE recommendations (
 id INTEGER PRIMARY KEY,
 media_id INTEGER NOT NULL REFERENCES media(id),
 sender_id INTEGER NOT NULL REFERENCES users(id),
 note TEXT NOT NULL DEFAULT '' CHECK(length(note) <= 2000),
 created_at INTEGER NOT NULL
);
CREATE TABLE recommendation_destinations (
 recommendation_id INTEGER PRIMARY KEY REFERENCES recommendations(id) ON DELETE CASCADE,
 user_id INTEGER NOT NULL REFERENCES users(id)
);
CREATE INDEX destinations_user ON recommendation_destinations(user_id, recommendation_id);
CREATE INDEX recommendations_sender ON recommendations(sender_id, id);
CREATE TABLE reactions (
 recommendation_id INTEGER NOT NULL REFERENCES recommendations(id) ON DELETE CASCADE,
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 listening TEXT NOT NULL DEFAULT 'unheard' CHECK(listening IN ('unheard','listened','revisit','dismissed')),
 rating INTEGER NOT NULL DEFAULT 0 CHECK(rating IN (-1,0,1)),
 note TEXT NOT NULL DEFAULT '' CHECK(length(note) <= 2000),
 updated_at INTEGER NOT NULL,
 PRIMARY KEY(recommendation_id, user_id)
);
CREATE TABLE comments (
 id INTEGER PRIMARY KEY,
 recommendation_id INTEGER NOT NULL REFERENCES recommendations(id) ON DELETE CASCADE,
 author_id INTEGER NOT NULL REFERENCES users(id),
 body TEXT NOT NULL CHECK(length(body) BETWEEN 1 AND 2000),
 created_at INTEGER NOT NULL
);
CREATE INDEX comments_recommendation ON comments(recommendation_id, id);
CREATE TABLE metadata_jobs (
 media_id INTEGER PRIMARY KEY REFERENCES media(id) ON DELETE CASCADE,
 attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 3),
 next_attempt INTEGER NOT NULL DEFAULT 0
);
