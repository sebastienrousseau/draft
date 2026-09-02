// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package runes provides UTF-8-safe cut points for the several places that
// bound a string by a byte budget.
//
// Slicing a string at an arbitrary byte offset can split a multi-byte rune in
// half. The halves normalise to U+FFFD, and claims.Verify rejects any quote
// containing one — so a carelessly cut prompt costs verified claims further
// down the run. internal/pdf already guarded its own cut this way; these are
// the same two operations, shared, so the next caller cannot get it wrong.
package runes

import "unicode/utf8"

// BoundaryAtOrBefore returns the largest index <= limit that starts a UTF-8
// rune, so s[:BoundaryAtOrBefore(s, limit)] can never split one in half.
func BoundaryAtOrBefore(s string, limit int) int {
	if limit >= len(s) {
		return len(s)
	}
	if limit <= 0 {
		return 0
	}
	// A UTF-8 continuation run is at most utf8.UTFMax-1 bytes long, so the
	// walk back is bounded. Index 0 must be included: a document opening with
	// a multi-byte rune has no earlier boundary to find, and stopping one
	// short of it is exactly the split this function exists to prevent.
	for i := limit; i > limit-utf8.UTFMax && i >= 0; i-- {
		if utf8.RuneStart(s[i]) {
			return i
		}
	}
	// Unreachable for well-formed UTF-8. On malformed input, cut nothing
	// rather than cut mid-rune.
	return 0
}

// BoundaryAtOrAfter returns the smallest index >= offset that starts a UTF-8
// rune, so s[BoundaryAtOrAfter(s, offset):] never begins mid-rune.
func BoundaryAtOrAfter(s string, offset int) int {
	if offset <= 0 {
		return 0
	}
	if offset >= len(s) {
		return len(s)
	}
	// A continuation run is at most utf8.UTFMax-1 bytes, so the scan is
	// bounded. Falling off the end means the trailing bytes are a partial
	// rune, and the only safe boundary left is the end of the string.
	for i := offset; i < len(s) && i < offset+utf8.UTFMax; i++ {
		if utf8.RuneStart(s[i]) {
			return i
		}
	}
	return len(s)
}

// CutHead returns the first limit bytes of s, backed up to a rune boundary.
func CutHead(s string, limit int) string { return s[:BoundaryAtOrBefore(s, limit)] }

// CutTail returns the last limit bytes of s, advanced to a rune boundary.
func CutTail(s string, limit int) string {
	if limit >= len(s) {
		return s
	}
	return s[BoundaryAtOrAfter(s, len(s)-limit):]
}
