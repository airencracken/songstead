// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import "context"

type Archive struct {
	Format          string           `json:"format"`
	Version         int              `json:"version"`
	User            User             `json:"user"`
	Recommendations []Recommendation `json:"recommendations"`
	Comments        []ExportComment  `json:"comments"`
}
type ExportComment struct {
	RecommendationID int64
	Comment
}

// Export takes a consistent snapshot of accessible recommendations, the user's
// reactions and personal notes, and comments they authored. Credentials and
// other people's private reactions never enter the archive types.
func (s *Store) Export(ctx context.Context, user User) (Archive, error) {
	archive := Archive{Format: "songstead-account", Version: 1, User: user, Recommendations: []Recommendation{}, Comments: []ExportComment{}}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return archive, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, selectRecommendation+" WHERE "+visible+" ORDER BY r.id", user.ID, user.ID, user.ID)
	if err != nil {
		return archive, err
	}
	for rows.Next() {
		item, err := scanRecommendation(rows)
		if err != nil {
			rows.Close()
			return archive, err
		}
		archive.Recommendations = append(archive.Recommendations, item)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return archive, err
	}
	if closeErr != nil {
		return archive, closeErr
	}
	rows, err = tx.QueryContext(ctx, `SELECT c.recommendation_id,c.id,c.author_id,c.created_at,u.username,c.body
 FROM comments c JOIN users u ON u.id=c.author_id WHERE c.author_id=? ORDER BY c.id`, user.ID)
	if err != nil {
		return archive, err
	}
	for rows.Next() {
		var c ExportComment
		if err := rows.Scan(&c.RecommendationID, &c.ID, &c.AuthorID, &c.CreatedAt, &c.Author, &c.Body); err != nil {
			rows.Close()
			return archive, err
		}
		archive.Comments = append(archive.Comments, c)
	}
	err = rows.Err()
	closeErr = rows.Close()
	if err != nil {
		return archive, err
	}
	if closeErr != nil {
		return archive, closeErr
	}
	return archive, tx.Commit()
}
