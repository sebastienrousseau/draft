// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import "testing"

func TestIsSingleToken(t *testing.T) {
	for in, want := range map[string]bool{
		"leverage": true, "leverages": true, "a1_b": true,
		"": false, "cutting-edge": false, "two words": false,
	} {
		if got := isSingleToken(in); got != want {
			t.Errorf("isSingleToken(%q) = %v, want %v", in, got, want)
		}
	}
}

// findAll must locate both a single-token form and a hyphenated form in one
// text, and return them in text order (the merge + sort path).
func TestBannedScannerFindAllMixed(t *testing.T) {
	s := newBannedScanner([]string{"leverage", "cutting-edge"})
	text := "we cutting-edge things then leverage them" // hyphenated first, single-token second
	hits := s.findAll(text)
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2: %v", len(hits), hits)
	}
	if text[hits[0][0]:hits[0][1]] != "cutting-edge" || text[hits[1][0]:hits[1][1]] != "leverage" {
		t.Errorf("hits out of order: %q, %q", text[hits[0][0]:hits[0][1]], text[hits[1][0]:hits[1][1]])
	}
}

// The set path is exactly a word-boundary match: a banned form embedded in a
// longer token is not a hit.
func TestBannedScannerRespectsWordBoundaries(t *testing.T) {
	s := newBannedScanner([]string{"leverage"})
	if hits := s.findAll("leveraged leveraging overleverage"); len(hits) != 0 {
		t.Errorf("substring matches leaked through: %v", hits)
	}
	if hits := s.findAll("a leverage here"); len(hits) != 1 {
		t.Errorf("a standalone form should match once, got %v", hits)
	}
}
