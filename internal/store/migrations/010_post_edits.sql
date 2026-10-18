-- Corrections belong to one recommendation, not every post of this music.
ALTER TABLE recommendations ADD COLUMN title_override TEXT CHECK(title_override IS NULL OR length(title_override) BETWEEN 1 AND 160);
ALTER TABLE recommendations ADD COLUMN artist_override TEXT CHECK(artist_override IS NULL OR length(artist_override)<=160);
ALTER TABLE recommendations ADD COLUMN kind_override TEXT CHECK(kind_override IS NULL OR kind_override IN ('link','track','album','artist'));
