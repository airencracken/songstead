CREATE VIRTUAL TABLE music_search USING fts5(text,tokenize='trigram case_sensitive 1');
INSERT INTO music_search(rowid,text) SELECT r.id,songstead_fold(coalesce(r.title_override,m.title)||char(10)||coalesce(r.artist_override,m.artist)||char(10)||r.note||char(10)||coalesce(l.genre,'')||char(10)||coalesce((SELECT group_concat(value,char(10)) FROM json_each(coalesce(l.tags,'[]'))),'')) FROM recommendations r JOIN media m ON m.id=r.media_id LEFT JOIN recommendation_labels l ON l.recommendation_id=r.id;
CREATE TRIGGER search_recommendations_insert AFTER INSERT ON recommendations BEGIN
DELETE FROM music_search WHERE rowid=NEW.id;
INSERT INTO music_search(rowid,text) SELECT r.id,songstead_fold(coalesce(r.title_override,m.title)||char(10)||coalesce(r.artist_override,m.artist)||char(10)||r.note||char(10)||coalesce(l.genre,'')||char(10)||coalesce((SELECT group_concat(value,char(10)) FROM json_each(coalesce(l.tags,'[]'))),'')) FROM recommendations r JOIN media m ON m.id=r.media_id LEFT JOIN recommendation_labels l ON l.recommendation_id=r.id WHERE r.id=NEW.id;
END;
CREATE TRIGGER search_recommendations_update AFTER UPDATE ON recommendations BEGIN
DELETE FROM music_search WHERE rowid=NEW.id;
INSERT INTO music_search(rowid,text) SELECT r.id,songstead_fold(coalesce(r.title_override,m.title)||char(10)||coalesce(r.artist_override,m.artist)||char(10)||r.note||char(10)||coalesce(l.genre,'')||char(10)||coalesce((SELECT group_concat(value,char(10)) FROM json_each(coalesce(l.tags,'[]'))),'')) FROM recommendations r JOIN media m ON m.id=r.media_id LEFT JOIN recommendation_labels l ON l.recommendation_id=r.id WHERE r.id=NEW.id;
END;
CREATE TRIGGER search_recommendations_delete AFTER DELETE ON recommendations BEGIN
DELETE FROM music_search WHERE rowid=OLD.id;
INSERT INTO music_search(rowid,text) SELECT r.id,songstead_fold(coalesce(r.title_override,m.title)||char(10)||coalesce(r.artist_override,m.artist)||char(10)||r.note||char(10)||coalesce(l.genre,'')||char(10)||coalesce((SELECT group_concat(value,char(10)) FROM json_each(coalesce(l.tags,'[]'))),'')) FROM recommendations r JOIN media m ON m.id=r.media_id LEFT JOIN recommendation_labels l ON l.recommendation_id=r.id WHERE r.id=OLD.id;
END;
CREATE TRIGGER search_recommendation_labels_insert AFTER INSERT ON recommendation_labels BEGIN
DELETE FROM music_search WHERE rowid=NEW.recommendation_id;
INSERT INTO music_search(rowid,text) SELECT r.id,songstead_fold(coalesce(r.title_override,m.title)||char(10)||coalesce(r.artist_override,m.artist)||char(10)||r.note||char(10)||coalesce(l.genre,'')||char(10)||coalesce((SELECT group_concat(value,char(10)) FROM json_each(coalesce(l.tags,'[]'))),'')) FROM recommendations r JOIN media m ON m.id=r.media_id LEFT JOIN recommendation_labels l ON l.recommendation_id=r.id WHERE r.id=NEW.recommendation_id;
END;
CREATE TRIGGER search_recommendation_labels_update AFTER UPDATE ON recommendation_labels BEGIN
DELETE FROM music_search WHERE rowid=NEW.recommendation_id;
INSERT INTO music_search(rowid,text) SELECT r.id,songstead_fold(coalesce(r.title_override,m.title)||char(10)||coalesce(r.artist_override,m.artist)||char(10)||r.note||char(10)||coalesce(l.genre,'')||char(10)||coalesce((SELECT group_concat(value,char(10)) FROM json_each(coalesce(l.tags,'[]'))),'')) FROM recommendations r JOIN media m ON m.id=r.media_id LEFT JOIN recommendation_labels l ON l.recommendation_id=r.id WHERE r.id=NEW.recommendation_id;
END;
CREATE TRIGGER search_recommendation_labels_delete AFTER DELETE ON recommendation_labels BEGIN
DELETE FROM music_search WHERE rowid=OLD.recommendation_id;
INSERT INTO music_search(rowid,text) SELECT r.id,songstead_fold(coalesce(r.title_override,m.title)||char(10)||coalesce(r.artist_override,m.artist)||char(10)||r.note||char(10)||coalesce(l.genre,'')||char(10)||coalesce((SELECT group_concat(value,char(10)) FROM json_each(coalesce(l.tags,'[]'))),'')) FROM recommendations r JOIN media m ON m.id=r.media_id LEFT JOIN recommendation_labels l ON l.recommendation_id=r.id WHERE r.id=OLD.recommendation_id;
END;
CREATE TRIGGER search_media_update AFTER UPDATE ON media BEGIN
DELETE FROM music_search WHERE rowid IN (SELECT id FROM recommendations WHERE media_id=NEW.id);
INSERT INTO music_search(rowid,text) SELECT r.id,songstead_fold(coalesce(r.title_override,m.title)||char(10)||coalesce(r.artist_override,m.artist)||char(10)||r.note||char(10)||coalesce(l.genre,'')||char(10)||coalesce((SELECT group_concat(value,char(10)) FROM json_each(coalesce(l.tags,'[]'))),'')) FROM recommendations r JOIN media m ON m.id=r.media_id LEFT JOIN recommendation_labels l ON l.recommendation_id=r.id WHERE r.media_id=NEW.id;
END;
