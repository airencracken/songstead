CREATE TABLE user_profiles (
 user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 still BLOB NOT NULL DEFAULT X'' CHECK(length(still)=0 OR (length(still) BETWEEN 8 AND 4194304 AND substr(still,1,8)=X'89504E470D0A1A0A')),
 animation BLOB NOT NULL DEFAULT X'' CHECK(length(animation)=0 OR (length(animation) BETWEEN 6 AND 4194304 AND substr(animation,1,6) IN (X'474946383761',X'474946383961'))),
 animate INTEGER NOT NULL DEFAULT 1 CHECK(animate IN (0,1)),
 CHECK(length(animation)=0 OR length(still)>0)
);
