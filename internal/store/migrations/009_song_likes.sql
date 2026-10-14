ALTER TABLE music_states ADD COLUMN liked INTEGER NOT NULL DEFAULT 0 CHECK(liked IN (0,1));

-- Preserve existing feedback. The latest recommendation reaction determines
-- the initial song like; listening status and per-recommendation notes stay put.
INSERT INTO music_states(media_id,user_id,listening,updated_at)
SELECT r.media_id,x.user_id,x.listening,x.updated_at
FROM reactions x JOIN recommendations r ON r.id=x.recommendation_id
WHERE x.recommendation_id=(
 SELECT x2.recommendation_id FROM reactions x2
 JOIN recommendations r2 ON r2.id=x2.recommendation_id
 WHERE r2.media_id=r.media_id AND x2.user_id=x.user_id
 ORDER BY x2.updated_at DESC,x2.recommendation_id DESC LIMIT 1
)
ON CONFLICT(media_id,user_id) DO NOTHING;

UPDATE music_states SET liked=coalesce((
 SELECT x.rating=1 FROM reactions x
 JOIN recommendations r ON r.id=x.recommendation_id
 WHERE r.media_id=music_states.media_id AND x.user_id=music_states.user_id
 ORDER BY x.updated_at DESC,x.recommendation_id DESC LIMIT 1
),0);

CREATE INDEX music_states_liked ON music_states(user_id,media_id) WHERE liked=1;
