// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import "context"

type ExportRecording struct {
	MediaID int64
	Recording
}
type ExportDiscussion struct {
	MediaID int64
	URL     string
}
type Archive struct {
	Profile              Profile              `json:"profile"`
	DiscoveryPreferences DiscoveryPreferences `json:"discovery_preferences"`
	AnnotationMode       string               `json:"annotation_mode"`
	Recordings           []ExportRecording    `json:"recordings"`
	Groups               []Group              `json:"groups"`
	Discussions          []ExportDiscussion   `json:"discussions"`
	Format               string               `json:"format"`
	Version              int                  `json:"version"`
	User                 User                 `json:"user"`
	Recommendations      []Recommendation     `json:"recommendations"`
	Comments             []ExportComment      `json:"comments"`
}
type ExportComment struct {
	RecommendationID int64
	Comment
}

// Export takes a consistent snapshot of accessible recommendations, the user's
// reactions and personal notes, and comments they authored. Credentials and
// other people's private reactions never enter the archive types.
func (s *Store) Export(ctx context.Context, user User) (Archive, error) {
	archive := Archive{Format: "songstead-account", Version: 4, User: user, Recommendations: []Recommendation{}, Comments: []ExportComment{}}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return archive, err
	}
	defer tx.Rollback()
	archive.Profile, err = readProfile(ctx, tx, user.ID)
	if err != nil {
		return archive, err
	}
	archive.DiscoveryPreferences, err = readDiscoveryPreferences(ctx, tx, user.ID)
	if err != nil {
		return archive, err
	}
	rows, err := tx.QueryContext(ctx, selectRecommendation+" WHERE "+visible+" ORDER BY r.id", user.ID, user.ID, user.ID, user.ID, user.ID)
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
	if err = tx.QueryRowContext(ctx, "SELECT annotation_mode FROM users WHERE id=?", user.ID).Scan(&archive.AnnotationMode); err != nil {
		return archive, err
	}
	for i := range archive.Comments {
		rows, err := tx.QueryContext(ctx, "SELECT recording_id,seconds,start_byte,end_byte FROM annotation_offsets WHERE comment_id=? ORDER BY ordinal", archive.Comments[i].ID)
		if err != nil {
			return archive, err
		}
		for rows.Next() {
			var o Offset
			if err = rows.Scan(&o.RecordingID, &o.Seconds, &o.Start, &o.End); err != nil {
				rows.Close()
				return archive, err
			}
			archive.Comments[i].Offsets = append(archive.Comments[i].Offsets, o)
			archive.Comments[i].RecordingID = o.RecordingID
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return archive, err
		}
	}
	seen := map[int64]bool{}
	for _, rec := range archive.Recommendations {
		if seen[rec.MediaID] {
			continue
		}
		seen[rec.MediaID] = true
		rows, err := tx.QueryContext(ctx, `SELECT mr.media_id,t.id,t.url,t.title,t.duration,coalesce(p.seconds,-1) FROM media_recordings mr JOIN recordings t ON t.id=mr.recording_id LEFT JOIN listening_positions p ON p.recording_id=t.id AND p.user_id=? WHERE mr.media_id=? ORDER BY t.id`, user.ID, rec.MediaID)
		if err != nil {
			return archive, err
		}
		for rows.Next() {
			var record ExportRecording
			if err = rows.Scan(&record.MediaID, &record.ID, &record.URL, &record.Title, &record.Duration, &record.Position); err != nil {
				rows.Close()
				return archive, err
			}
			archive.Recordings = append(archive.Recordings, record)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return archive, err
		}
	}
	rows, err = tx.QueryContext(ctx, "SELECT g.id,g.owner_id,g.name FROM groups g JOIN group_members m ON m.group_id=g.id WHERE m.user_id=? ORDER BY g.id", user.ID)
	if err != nil {
		return archive, err
	}
	for rows.Next() {
		var group Group
		if err = rows.Scan(&group.ID, &group.OwnerID, &group.Name); err != nil {
			rows.Close()
			return archive, err
		}
		archive.Groups = append(archive.Groups, group)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return archive, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT media_id,url FROM discussions WHERE user_id=? ORDER BY media_id,url", user.ID)
	if err != nil {
		return archive, err
	}
	for rows.Next() {
		var link ExportDiscussion
		if err = rows.Scan(&link.MediaID, &link.URL); err != nil {
			rows.Close()
			return archive, err
		}
		archive.Discussions = append(archive.Discussions, link)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return archive, err
	}
	return archive, tx.Commit()
}
