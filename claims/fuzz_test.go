// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"strings"
	"testing"
)

// FuzzParse drives the grounding gate with arbitrary model output. The
// security-critical invariant is that a surviving claim's quote must appear
// verbatim in the source: everything downstream treats the ledger as fact, so
// a quote that is not in the source is a fabrication that reached the writer.
func FuzzParse(f *testing.F) {
	f.Add("CLAIM: A scored 0.82\nSOURCE_QUOTE: \"scored 0.82\"\nTYPE: metric\nSTRENGTH: demonstrated\n---",
		"The system scored 0.82 on the held-out set.")
	f.Add("NONE", "some source")
	f.Add("", "")
	f.Add("CLAIM: x\nSOURCE_QUOTE: \"not present\"\nTYPE: result\nSTRENGTH: claimed\n---", "unrelated text")
	f.Add("---\n---\n---", "s")
	f.Add("CLAIM: 5x faster\nSOURCE_QUOTE: \"5x\"\nTYPE: metric\nSTRENGTH: demonstrated\n---", "it was 5x")

	f.Fuzz(func(t *testing.T, text, source string) {
		records, dropped := Parse(text, source)
		if dropped < 0 {
			t.Fatalf("negative dropped count %d", dropped)
		}
		for _, r := range records {
			if r.SourceQuote == "" {
				t.Errorf("verified record has an empty quote: %+v", r)
			}
			if !groundedIn(source, r.SourceQuote) {
				t.Errorf("ungrounded quote survived verification:\nquote:  %q\nsource: %q", r.SourceQuote, source)
			}
			if r.Claim == "" {
				t.Errorf("verified record has an empty claim: %+v", r)
			}
		}
	})
}

// groundedIn reports whether quote genuinely occurs in source.
//
// The bar is deliberately not byte-exact containment. Text arrives from
// pdftotext, where a sentence is wrapped across lines and columns are separated
// by runs of spaces, so a quote that differs from the source only in how much
// whitespace sits between its words IS present in that source — and refusing it
// would drop true claims from every real paper. Case and curly quotes are
// tolerated for the same reason.
//
// What it must still catch is fabrication: content the source does not contain.
// Collapsing runs of whitespace cannot manufacture that, because a run collapses
// to one space and never to none — "a b" does not match "ab", nor "ab" match
// "a b".
//
// This is written independently of the production normalise() so that the test
// checks the property rather than restating the implementation.
func groundedIn(source, quote string) bool {
	fold := func(s string) string {
		s = strings.NewReplacer("“", `"`, "”", `"`, "‘", "'", "’", "'").Replace(s)
		return strings.ToLower(strings.Join(strings.Fields(s), " "))
	}
	return strings.Contains(fold(source), fold(quote))
}

// The loosened invariant must still reject fabrication: collapsing whitespace
// tolerates spacing, never invented content.
func TestGroundedInStillRejectsFabrication(t *testing.T) {
	for _, tc := range []struct {
		name, source, quote string
		want                bool
	}{
		{name: "exact", source: "used 5x fewer FLOPs", quote: "used 5x fewer FLOPs", want: true},
		{name: "wrapped across a line", source: "used 5x\nfewer FLOPs", quote: "used 5x fewer FLOPs", want: true},
		{name: "column gutter", source: "used 5x    fewer FLOPs", quote: "used 5x fewer FLOPs", want: true},
		{name: "case differs", source: "Used 5X Fewer FLOPs", quote: "used 5x fewer flops", want: true},
		{name: "curly apostrophe", source: "the model’s output", quote: "the model's output", want: true},
		{name: "words joined is not a match", source: "ab", quote: "a b", want: false},
		{name: "words split is not a match", source: "a b", quote: "ab", want: false},
		{name: "invented number", source: "used 5x fewer FLOPs", quote: "used 9x fewer FLOPs", want: false},
		{name: "invented clause", source: "used 5x fewer FLOPs", quote: "and halved training cost", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := groundedIn(tc.source, tc.quote); got != tc.want {
				t.Errorf("groundedIn(%q, %q) = %v, want %v", tc.source, tc.quote, got, tc.want)
			}
		})
	}
}
