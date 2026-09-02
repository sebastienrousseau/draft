// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package mdspan

import (
	"strings"
	"testing"
)

func TestProtectedFindsEachKind(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"fenced", "a\n```\nleverage\n```\nb", "```\nleverage\n```"},
		{"tilde fence", "a\n~~~\nleverage\n~~~\nb", "~~~\nleverage\n~~~"},
		{"inline code", "a `leverage` b", "`leverage`"},
		{"straight quote", `a "leverage" b`, `"leverage"`},
		{"smart quote", "a “leverage” b", "“leverage”"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			locs := Protected(tc.in)
			if len(locs) != 1 {
				t.Fatalf("Protected(%q) = %d spans, want 1", tc.in, len(locs))
			}
			if got := tc.in[locs[0][0]:locs[0][1]]; got != tc.want {
				t.Errorf("span = %q, want %q", got, tc.want)
			}
		})
	}
}

// A blockquote is the article's own Executive Summary in this house style, so
// exempting it would silence the vocabulary rules on original prose.
func TestProtectedIgnoresBlockquotes(t *testing.T) {
	if locs := Protected("> Executive Summary: we leverage things.\n"); len(locs) != 0 {
		t.Errorf("Protected() found %d spans in a blockquote, want 0", len(locs))
	}
}

// An unbalanced quote must not swallow the document and exempt it wholesale.
func TestProtectedBoundsAnUnbalancedQuote(t *testing.T) {
	in := `He said "and then never closed the quote` + "\n" + strings.Repeat("leverage ", 50)
	if locs := Protected(in); len(locs) != 0 {
		t.Errorf("Protected() = %d spans for an unbalanced quote, want 0", len(locs))
	}
	long := `"` + strings.Repeat("x", 500) + `"`
	if locs := Protected(long); len(locs) != 0 {
		t.Errorf("Protected() matched an over-long quoted span")
	}
}

func TestOutsideProtectedSplicesSpansBack(t *testing.T) {
	in := `up "keep" down`
	got := OutsideProtected(in, func(s string) string { return strings.ToUpper(s) })
	if want := `UP "keep" DOWN`; got != want {
		t.Errorf("OutsideProtected() = %q, want %q", got, want)
	}
}

func TestOutsideProtectedWithNoSpansAppliesToAll(t *testing.T) {
	got := OutsideProtected("plain text", func(s string) string { return strings.ToUpper(s) })
	if want := "PLAIN TEXT"; got != want {
		t.Errorf("OutsideProtected() = %q, want %q", got, want)
	}
}

// Every protected span opens with one of four characters; a draft carrying
// none of them must not pay for the alternation scan.
func TestProtectedSkipsDocumentsWithNoDelimiters(t *testing.T) {
	if locs := Protected("plain prose with no code and no quotation at all"); locs != nil {
		t.Errorf("Protected() = %v, want nil", locs)
	}
	// The pre-check must not swallow a real span.
	if locs := Protected("a ~~~\nx\n~~~ b"); len(locs) != 1 {
		t.Errorf("Protected() found %d spans, want 1", len(locs))
	}
}

func TestCovers(t *testing.T) {
	s := `a "one" b "two" c`
	spans := Protected(s)
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	for _, tc := range []struct {
		offset int
		want   bool
	}{
		{0, false},              // 'a'
		{spans[0][0], true},     // opening quote
		{spans[0][1] - 1, true}, // closing quote
		{spans[0][1], false},    // just past it
		{spans[1][0], true},     // second span
		{len(s) - 1, false},     // 'c'
	} {
		if got := Covers(spans, tc.offset); got != tc.want {
			t.Errorf("Covers(spans, %d) = %v, want %v", tc.offset, got, tc.want)
		}
	}
	if Covers(nil, 0) {
		t.Error("Covers(nil, 0) = true")
	}
}
