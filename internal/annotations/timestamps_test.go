// SPDX-License-Identifier: AGPL-3.0-or-later

package annotations

import (
	"strings"
	"testing"
)

func TestNaturalTimestamps(t *testing.T) {
	body := "That bass at 2:43, then 1:02:03 and 0:00."
	refs := Parse(body, 0)
	want := []int{163, 3723, 0}
	if len(refs) != len(want) {
		t.Fatal(refs)
	}
	for i, r := range refs {
		if r.Seconds != want[i] {
			t.Fatal(r)
		}
		n, ok := Seconds(body[r.Start:r.End])
		if !ok || n != r.Seconds {
			t.Fatal(r)
		}
	}
}
func TestInvalidAmbiguousAndDuration(t *testing.T) {
	for _, s := range []string{"12:99", "1:99:05", "1:2", "1:00:99", "word2:43", "2:43word", "https://host/2:43", "https://host?2:43", "https://host/@2:43", "at #2:43", "2:43:00:00", "999999:00"} {
		if got := Parse(s, 0); len(got) != 0 {
			t.Errorf("%q: %+v", s, got)
		}
	}
	if got := Parse("2:43 and 2:44", 163); len(got) != 1 || got[0].Seconds != 163 {
		t.Fatal(got)
	}
}
func FuzzTimestampOffsets(f *testing.F) {
	f.Add("α 2:43 1:59:59")
	f.Fuzz(func(t *testing.T, s string) {
		for _, r := range Parse(s, 3600) {
			if r.Start < 0 || r.End > len(s) || r.Seconds > 3600 {
				t.Fatal(r)
			}
			n, ok := Seconds(s[r.Start:r.End])
			if !ok || n != r.Seconds {
				t.Fatal(r)
			}
		}
	})
}
func TestPositionRoundTrip(t *testing.T) {
	for n := 0; n <= 86400; n += 7 {
		got, ok := Seconds(Format(n))
		if !ok || got != n {
			t.Fatal(n, got)
		}
	}
	if _, ok := Seconds(strings.Repeat("9", 200)); ok {
		t.Fatal("overflow")
	}
}
