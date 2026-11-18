// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package runes

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// StripInvisible removes the characters that display as nothing yet still
// reach a model that reads the text: Unicode tag characters
// (U+E0000-U+E007F), which spell out ASCII invisibly; bidirectional
// embeddings, overrides and isolates (U+202A-U+202E, U+2066-U+2069), which
// make the displayed order differ from the stored one; and the zero-width
// space, word joiner and byte-order mark (U+200B, U+2060, U+FEFF).
//
// The zero-width non-joiner and joiner (U+200C, U+200D) are different: between
// two letters of Persian, Arabic or an Indic script, or inside an emoji
// sequence, they change how the text is written, and a quote of those words
// has to keep them to match. So each is kept when both its neighbours are
// visible non-ASCII characters, and removed anywhere else, which is where a
// hidden payload or a split keyword puts them: inside ASCII words, beside
// spaces, or in runs.
//
// Directional marks (U+200E, U+200F, U+061C) are kept. They are how
// right-to-left text is written correctly, and they cannot reorder anything
// beyond the character next to them.
//
// Text with nothing to remove is returned as it is, malformed bytes and all.
// Once something is removed, every byte that is not valid UTF-8 becomes
// U+FFFD: removing a character from between two stray fragments would
// otherwise join them into a new character, which could be the very kind
// just removed. claims.Verify refuses a quote holding U+FFFD, so no claim can
// rest on that text either way.
func StripInvisible(s string) string {
	first := firstInvisible(s)
	if first < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	b.WriteString(strings.ToValidUTF8(s[:first], string(utf8.RuneError)))
	prev, _ := utf8.DecodeLastRuneInString(s[:first])
	for i := first; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])
		if keepRune(r, prev, s[i+w:]) {
			b.WriteRune(r)
		}
		// The neighbour is the original one, so a run of joiners is judged
		// against its own members and removed whole.
		prev = r
		i += w
	}
	return b.String()
}

// firstInvisible is the offset of the first invisible character in s, or -1.
//
// It runs over every source document, almost all of which have none, so it
// looks at bytes rather than decoding runes: every character it removes is
// encoded with a lead byte of 0xE2, 0xEF or 0xF3, and only those are decoded.
// That keeps the clean case close to a plain scan.
func firstInvisible(s string) int {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case 0xE2, 0xEF, 0xF3:
			if r, _ := utf8.DecodeRuneInString(s[i:]); isInvisible(r) {
				return i
			}
		}
	}
	return -1
}

// The two joiners, written as numbers so the source holds no invisible text.
const (
	zwnj = 0x200C
	zwj  = 0x200D
)

// keepRune decides r given the rune before it and the text after it.
func keepRune(r, prev rune, after string) bool {
	if !isInvisible(r) {
		return true
	}
	if r != zwnj && r != zwj {
		return false
	}
	next, _ := utf8.DecodeRuneInString(after)
	return joins(prev) && joins(next)
}

// joins reports a neighbour a joiner may legitimately sit beside.
func joins(r rune) bool {
	return r >= utf8.RuneSelf && r != utf8.RuneError && !isInvisible(r) && !unicode.IsSpace(r)
}

func isInvisible(r rune) bool {
	switch {
	case r >= 0xE0000 && r <= 0xE007F: // tag characters
		return true
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069: // bidi controls
		return true
	case r >= 0x200B && r <= 0x200D, r == 0x2060, r == 0xFEFF: // zero-width
		return true
	}
	return false
}
