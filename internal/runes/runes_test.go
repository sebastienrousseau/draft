// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package runes

import (
	"strings"
	"testing"
	"unicode/utf8"
)

const mixed = "aé測" // 1 + 2 + 3 bytes = 6

func TestBoundaryAtOrBefore(t *testing.T) {
	for _, tc := range []struct{ limit, want int }{
		{-1, 0},
		{0, 0},
		{1, 1}, // 'é' starts here
		{2, 1}, // mid-'é' backs up
		{3, 3}, // '測' starts here
		{4, 3}, // mid-'測' backs up
		{5, 3},
		{6, 6},  // exactly len
		{99, 6}, // past the end
	} {
		if got := BoundaryAtOrBefore(mixed, tc.limit); got != tc.want {
			t.Errorf("BoundaryAtOrBefore(%q, %d) = %d, want %d", mixed, tc.limit, got, tc.want)
		}
	}
}

func TestBoundaryAtOrAfter(t *testing.T) {
	for _, tc := range []struct{ offset, want int }{
		{-1, 0},
		{0, 0},
		{1, 1},
		{2, 3}, // mid-'é' advances to '測'
		{3, 3},
		{4, 6}, // mid-'測': no further start, so the end
		{5, 6},
		{6, 6},
		{99, 6},
	} {
		if got := BoundaryAtOrAfter(mixed, tc.offset); got != tc.want {
			t.Errorf("BoundaryAtOrAfter(%q, %d) = %d, want %d", mixed, tc.offset, got, tc.want)
		}
	}
}

// The property that matters: no cut at any offset may produce invalid UTF-8.
func TestCutsNeverSplitARune(t *testing.T) {
	s := strings.Repeat("字é a", 200)
	for i := -2; i <= len(s)+2; i++ {
		if head := CutHead(s, i); !utf8.ValidString(head) {
			t.Fatalf("CutHead(s, %d) produced invalid UTF-8", i)
		}
		if tail := CutTail(s, i); !utf8.ValidString(tail) {
			t.Fatalf("CutTail(s, %d) produced invalid UTF-8", i)
		}
	}
}

func TestCutsRespectTheirBudget(t *testing.T) {
	s := strings.Repeat("測", 10) // 30 bytes
	if got := CutHead(s, 10); len(got) > 10 {
		t.Errorf("CutHead exceeded its budget: %d bytes", len(got))
	}
	if got := CutTail(s, 10); len(got) > 10 {
		t.Errorf("CutTail exceeded its budget: %d bytes", len(got))
	}
	if got := CutHead(s, 999); got != s {
		t.Errorf("CutHead truncated a string shorter than the budget")
	}
	if got := CutTail(s, 999); got != s {
		t.Errorf("CutTail truncated a string shorter than the budget")
	}
}

func TestCutTailKeepsTheEnd(t *testing.T) {
	if got := CutTail("abcdef", 3); got != "def" {
		t.Errorf("CutTail = %q, want %q", got, "def")
	}
}

// Malformed input has no boundary to find. Cutting nothing is the safe answer;
// the alternative is handing on a half rune.
func TestBoundariesOnMalformedInput(t *testing.T) {
	bad := "\x80\x80\x80\x80\x80\x80"
	if got := BoundaryAtOrBefore(bad, 5); got != 0 {
		t.Errorf("BoundaryAtOrBefore(malformed, 5) = %d, want 0", got)
	}
	if got := BoundaryAtOrAfter(bad, 1); got != len(bad) {
		t.Errorf("BoundaryAtOrAfter(malformed, 1) = %d, want %d", got, len(bad))
	}
}

// A document that opens with a multi-byte rune has no earlier boundary, which
// is the case the walk-back must not stop one byte short of.
func TestCutHeadAtStartOfMultiByteRune(t *testing.T) {
	if got := CutHead("測abc", 1); got != "" {
		t.Errorf("CutHead = %q, want %q", got, "")
	}
	if got := CutHead("測abc", 3); got != "測" {
		t.Errorf("CutHead = %q, want %q", got, "測")
	}
}
