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

func TestBlankProtectedKeepsLengthAndNewlines(t *testing.T) {
	in := "a\n```\nx\n```\nb"
	got := BlankProtected(in)
	if len(got) != len(in) {
		t.Errorf("BlankProtected() len = %d, want %d", len(got), len(in))
	}
	if strings.Count(got, "\n") != strings.Count(in, "\n") {
		t.Errorf("BlankProtected() changed the line count")
	}
	if strings.Contains(got, "x") {
		t.Errorf("BlankProtected() left protected content: %q", got)
	}
}

func TestBlankProtectedWithNoSpansIsIdentity(t *testing.T) {
	if got := BlankProtected("plain"); got != "plain" {
		t.Errorf("BlankProtected() = %q, want %q", got, "plain")
	}
}
