// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/rules"
	"github.com/sebastienrousseau/draft/validate"
)

// violationKeys parses two validator message shapes. If validate's wording
// changes, the comparison silently degrades to exact matching and --review
// starts blaming edits for problems they did not cause. Fail here instead.
func TestViolationMessageShapesAreStable(t *testing.T) {
	t.Run("word count", func(t *testing.T) {
		short := "# T\n\nToo short."
		var got string
		for _, e := range validate.Errors(short) {
			if strings.Contains(e, "words") {
				got = e
			}
		}
		if got == "" {
			t.Fatal("no word-count violation produced")
		}
		if !wordCountPat.MatchString(got) {
			t.Errorf("word-count message %q no longer matches %s", got, wordCountPat)
		}
	})

	t.Run("banned words", func(t *testing.T) {
		if len(rules.BannedWords) == 0 {
			t.Skip("no banned words configured")
		}
		md := "# T\n\n" + rules.BannedWords[0] + " appears here."
		var got string
		for _, e := range validate.Errors(md) {
			if strings.HasPrefix(e, "contains banned words") {
				got = e
			}
		}
		if got == "" {
			t.Fatalf("no banned-word violation produced for %q", rules.BannedWords[0])
		}
		if !bannedListPat.MatchString(got) {
			t.Errorf("banned-word message %q no longer matches %s", got, bannedListPat)
		}
	})
}

func TestViolationKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  string
		want []string
	}{
		{
			name: "a fixed message is its own key",
			msg:  "missing Executive Summary",
			want: []string{"missing Executive Summary"},
		},
		{
			name: "word count keys on the bound, not the number",
			msg:  "article is 4038 words; maximum is 3000",
			want: []string{"article word count violates the maximum"},
		},
		{
			name: "the minimum is a different key from the maximum",
			msg:  "article is 12 words; minimum is 500",
			want: []string{"article word count violates the minimum"},
		},
		{
			name: "a banned list becomes one key per item",
			msg:  "contains banned words: leverage, utilise",
			want: []string{"contains banned words: leverage", "contains banned words: utilise"},
		},
		{
			name: "banned phrases too",
			msg:  "contains banned phrases: at the end of the day",
			want: []string{"contains banned phrases: at the end of the day"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := violationKeys(tc.msg)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("key %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestNewViolations(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after []string
		want          []string
	}{
		{
			name:   "nothing new",
			before: []string{"article is 4039 words; maximum is 3000"},
			after:  []string{"article is 4038 words; maximum is 3000"},
			want:   nil,
		},
		{
			name:   "a genuinely new problem",
			before: []string{"article is 4039 words; maximum is 3000"},
			after:  []string{"article is 4038 words; maximum is 3000", "missing Executive Summary"},
			want:   []string{"missing Executive Summary"},
		},
		{
			name:   "an added banned word is caught despite a pre-existing one",
			before: []string{"contains banned words: leverage"},
			after:  []string{"contains banned words: leverage, utilise"},
			want:   []string{"contains banned words: leverage, utilise"},
		},
		{
			name:   "a pre-existing banned word alone is not blamed",
			before: []string{"contains banned words: leverage"},
			after:  []string{"contains banned words: leverage"},
			want:   nil,
		},
		{
			name:   "going from under to over length is a new problem",
			before: []string{"article is 12 words; minimum is 500"},
			after:  []string{"article is 4038 words; maximum is 3000"},
			want:   []string{"article is 4038 words; maximum is 3000"},
		},
		{
			name:   "a fixed problem is not reported",
			before: []string{"missing Executive Summary"},
			after:  nil,
			want:   nil,
		},
		{
			name:   "everything is new when there was nothing before",
			before: nil,
			after:  []string{"contains emoji"},
			want:   []string{"contains emoji"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := newViolations(tc.before, tc.after)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("entry %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
