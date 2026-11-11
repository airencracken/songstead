// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkSearchMusicScale(b *testing.B) {
	for _, n := range []int{1000, 10000, 100000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			s, u := fixture(b)
			ctx := context.Background()
			_, err := s.db.Exec(`WITH RECURSIVE seq(x) AS(SELECT 1 UNION ALL SELECT x+1 FROM seq WHERE x<?) INSERT INTO media(id,original_url,provider,title,artist,canonical_key) SELECT x,'https://example.org/'||x,'unknown','Morning music '||x,'The garden band','bench:'||x FROM seq`, n)
			if err != nil {
				b.Fatal(err)
			}
			_, err = s.db.Exec(`INSERT INTO recommendations(id,media_id,sender_id,note,created_at) SELECT id,id,?,'A song for today',id FROM media`, u[0])
			if err != nil {
				b.Fatal(err)
			}
			_, err = s.db.Exec(`INSERT INTO recommendation_destinations(recommendation_id,user_id) SELECT id,? FROM recommendations`, u[1])
			if err != nil {
				b.Fatal(err)
			}
			for _, query := range []string{"morning", "absent phrase"} {
				b.Run(query, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						_, err := s.SearchMusic(ctx, u[1], query, 0, 30)
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
