ALTER TABLE users ADD COLUMN suspended INTEGER NOT NULL DEFAULT 0 CHECK(suspended IN (0,1));
ALTER TABLE users ADD COLUMN can_invite INTEGER NOT NULL DEFAULT 0 CHECK(can_invite IN (0,1));
ALTER TABLE users ADD COLUMN invited_by INTEGER REFERENCES users(id);
CREATE TABLE instance_settings (id INTEGER PRIMARY KEY CHECK(id=1), content TEXT NOT NULL CHECK(json_valid(content)));
CREATE TABLE invitations (
 id INTEGER PRIMARY KEY,
 token_hash TEXT NOT NULL UNIQUE CHECK(length(token_hash)=64),
 prefix TEXT NOT NULL CHECK(length(prefix)=12),
 creator_id INTEGER NOT NULL REFERENCES users(id),
 label TEXT NOT NULL CHECK(length(label)<=64),
 created_at INTEGER NOT NULL,
 expires_at INTEGER NOT NULL CHECK(expires_at>=0),
 max_uses INTEGER NOT NULL CHECK(max_uses BETWEEN 0 AND 10000),
 uses INTEGER NOT NULL DEFAULT 0 CHECK(uses>=0 AND (max_uses=0 OR uses<=max_uses)),
 revoked INTEGER NOT NULL DEFAULT 0 CHECK(revoked IN (0,1))
);
CREATE TABLE password_resets (
 user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 token_hash TEXT NOT NULL UNIQUE CHECK(length(token_hash)=64),
 password_hash TEXT NOT NULL,
 expires_at INTEGER NOT NULL
);
CREATE TABLE branding_assets (name TEXT PRIMARY KEY CHECK(name IN ('mascot','favicon')), content BLOB NOT NULL CHECK(length(content)>0));
