CREATE TABLE media_artwork (
 media_id INTEGER PRIMARY KEY REFERENCES media(id) ON DELETE CASCADE,
 content BLOB NOT NULL CHECK(typeof(content)='blob' AND length(content) BETWEEN 8 AND 4194304 AND substr(content,1,8)=x'89504E470D0A1A0A')
);
CREATE TABLE recommendation_labels (
 recommendation_id INTEGER PRIMARY KEY REFERENCES recommendations(id) ON DELETE CASCADE,
 genre TEXT NOT NULL DEFAULT '' CHECK(length(genre)<=80 AND instr(genre,char(0))=0 AND instr(genre,char(10))=0 AND instr(genre,char(13))=0 AND instr(genre,char(9))=0 AND instr(genre,',')=0),
 genre_key TEXT NOT NULL DEFAULT '' CHECK(length(genre_key)<=80),
 tags TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(tags) AND json_type(tags)='array' AND json_array_length(tags)<=20 AND length(tags)<=4096),
 tag_keys TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(tag_keys) AND json_type(tag_keys)='array' AND json_array_length(tag_keys)<=20 AND length(tag_keys)<=4096)
);
CREATE TABLE discovery_preferences (
 user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 excluded_genres TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(excluded_genres) AND json_type(excluded_genres)='array' AND json_array_length(excluded_genres)<=20 AND length(excluded_genres)<=4096),
 preferred_genres TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(preferred_genres) AND json_type(preferred_genres)='array' AND json_array_length(preferred_genres)<=20 AND length(preferred_genres)<=4096),
 excluded_tags TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(excluded_tags) AND json_type(excluded_tags)='array' AND json_array_length(excluded_tags)<=20 AND length(excluded_tags)<=4096),
 preferred_tags TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(preferred_tags) AND json_type(preferred_tags)='array' AND json_array_length(preferred_tags)<=20 AND length(preferred_tags)<=4096)
);

CREATE TRIGGER labels_insert_guard BEFORE INSERT ON recommendation_labels BEGIN
 SELECT RAISE(ABORT,'invalid tag') WHERE EXISTS(SELECT 1 FROM json_each(NEW.tags) WHERE type!='text' OR length(value) NOT BETWEEN 1 AND 40 OR instr(value,char(0))>0 OR instr(value,char(10))>0 OR instr(value,char(13))>0 OR instr(value,char(9))>0 OR instr(value,',')>0);
 SELECT RAISE(ABORT,'invalid tag key') WHERE EXISTS(SELECT 1 FROM json_each(NEW.tag_keys) WHERE type!='text' OR length(value) NOT BETWEEN 1 AND 40);
END;
CREATE TRIGGER labels_update_guard BEFORE UPDATE ON recommendation_labels BEGIN
 SELECT RAISE(ABORT,'invalid tag') WHERE EXISTS(SELECT 1 FROM json_each(NEW.tags) WHERE type!='text' OR length(value) NOT BETWEEN 1 AND 40 OR instr(value,char(0))>0 OR instr(value,char(10))>0 OR instr(value,char(13))>0 OR instr(value,char(9))>0 OR instr(value,',')>0);
 SELECT RAISE(ABORT,'invalid tag key') WHERE EXISTS(SELECT 1 FROM json_each(NEW.tag_keys) WHERE type!='text' OR length(value) NOT BETWEEN 1 AND 40);
END;
CREATE TRIGGER discovery_insert_guard BEFORE INSERT ON discovery_preferences BEGIN
 SELECT RAISE(ABORT,'invalid genre preference') WHERE EXISTS(SELECT 1 FROM json_each(NEW.excluded_genres) WHERE type!='text' OR length(value) NOT BETWEEN 1 AND 80 OR instr(value,char(0))>0 OR instr(value,char(10))>0 OR instr(value,char(13))>0 OR instr(value,char(9))>0 OR instr(value,',')>0) OR EXISTS(SELECT 1 FROM json_each(NEW.preferred_genres) WHERE type!='text' OR length(value) NOT BETWEEN 1 AND 80 OR instr(value,char(0))>0 OR instr(value,char(10))>0 OR instr(value,char(13))>0 OR instr(value,char(9))>0 OR instr(value,',')>0);
 SELECT RAISE(ABORT,'invalid tag preference') WHERE EXISTS(SELECT 1 FROM json_each(NEW.excluded_tags) WHERE type!='text' OR length(value) NOT BETWEEN 1 AND 40 OR instr(value,char(0))>0 OR instr(value,char(10))>0 OR instr(value,char(13))>0 OR instr(value,char(9))>0 OR instr(value,',')>0) OR EXISTS(SELECT 1 FROM json_each(NEW.preferred_tags) WHERE type!='text' OR length(value) NOT BETWEEN 1 AND 40 OR instr(value,char(0))>0 OR instr(value,char(10))>0 OR instr(value,char(13))>0 OR instr(value,char(9))>0 OR instr(value,',')>0);
END;
CREATE TRIGGER discovery_update_guard BEFORE UPDATE ON discovery_preferences BEGIN
 SELECT RAISE(ABORT,'invalid genre preference') WHERE EXISTS(SELECT 1 FROM json_each(NEW.excluded_genres) WHERE type!='text' OR length(value) NOT BETWEEN 1 AND 80 OR instr(value,char(0))>0 OR instr(value,char(10))>0 OR instr(value,char(13))>0 OR instr(value,char(9))>0 OR instr(value,',')>0) OR EXISTS(SELECT 1 FROM json_each(NEW.preferred_genres) WHERE type!='text' OR length(value) NOT BETWEEN 1 AND 80 OR instr(value,char(0))>0 OR instr(value,char(10))>0 OR instr(value,char(13))>0 OR instr(value,char(9))>0 OR instr(value,',')>0);
 SELECT RAISE(ABORT,'invalid tag preference') WHERE EXISTS(SELECT 1 FROM json_each(NEW.excluded_tags) WHERE type!='text' OR length(value) NOT BETWEEN 1 AND 40 OR instr(value,char(0))>0 OR instr(value,char(10))>0 OR instr(value,char(13))>0 OR instr(value,char(9))>0 OR instr(value,',')>0) OR EXISTS(SELECT 1 FROM json_each(NEW.preferred_tags) WHERE type!='text' OR length(value) NOT BETWEEN 1 AND 40 OR instr(value,char(0))>0 OR instr(value,char(10))>0 OR instr(value,char(13))>0 OR instr(value,char(9))>0 OR instr(value,',')>0);
END;
