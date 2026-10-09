// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import "context"

// RecentComment contains only the shared conversation's public-to-members fields.
// Ratings, listening state and personal notes never enter this projection.
type RecentComment struct {
	Comment
	RecommendationID int64
	Title, Artist    string
	HasArtwork       bool
}

// RecentComments follows Recent's explicit instance audience. Apply spoiler
// rules before LIMIT so concealed comments cannot crowd out a page or disclose
// their existence through pagination. Comment IDs give stable append order even
// when new comments arrive between pages or the wall clock changes.
func (s *Store) RecentComments(ctx context.Context, viewer, before int64, limit int) ([]RecentComment, error) {
	if viewer <= 0 || before < 0 || limit < 1 || limit > 100 {
		return nil, ErrInvalid
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.author_id,c.created_at,u.username,c.body,r.id,m.title,m.artist,
 EXISTS(SELECT 1 FROM media_artwork a WHERE a.media_id=m.id)
 FROM comments c
 JOIN recommendations r ON r.id=c.recommendation_id
 JOIN recommendation_destinations d ON d.recommendation_id=r.id
 JOIN media m ON m.id=r.media_id
 JOIN users u ON u.id=c.author_id
 JOIN users v ON v.id=? AND v.suspended=0
 WHERE r.visibility='members' AND `+visible+`
 AND (?=0 OR c.id<?)
 AND NOT EXISTS (
  SELECT 1 FROM annotation_offsets o WHERE o.comment_id=c.id
  AND v.annotation_mode<>'immediate'
  AND (v.annotation_mode<>'spoiler-free' OR NOT EXISTS (
   SELECT 1 FROM listening_positions p WHERE p.user_id=v.id
   AND p.recording_id=o.recording_id AND p.seconds>=o.seconds
  ))
 )
 ORDER BY c.id DESC LIMIT ?`, viewer, viewer, viewer, viewer, before, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecentComment{}
	for rows.Next() {
		var c RecentComment
		if err := rows.Scan(&c.ID, &c.AuthorID, &c.CreatedAt, &c.Author, &c.Body, &c.RecommendationID, &c.Title, &c.Artist, &c.HasArtwork); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
