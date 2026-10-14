// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"time"
)

// SetLiked changes only the viewer's song like. A single statement checks
// recommendation access and saves the choice without changing other feedback.
func (s *Store) SetLiked(ctx context.Context, viewer, id int64, liked bool) error {
	result, err := s.db.ExecContext(ctx, `INSERT INTO music_states(media_id,user_id,listening,updated_at,liked)
 SELECT r.media_id,?,'unheard',?,? FROM recommendations r
 JOIN recommendation_destinations d ON d.recommendation_id=r.id
 WHERE r.id=? AND `+visible+` AND EXISTS(SELECT 1 FROM users v WHERE v.id=? AND v.suspended=0)
 ON CONFLICT(media_id,user_id) DO UPDATE SET liked=excluded.liked,updated_at=excluded.updated_at`,
		viewer, time.Now().Unix(), liked, id, viewer, viewer, viewer, viewer)
	return mutationResult(result, err)
}
