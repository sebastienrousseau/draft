// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package runes

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzStripInvisible holds StripInvisible to what NormaliseSpace relies on:
// nothing it always removes survives, it never adds a character (a stray byte
// may become U+FFFD, one for one), and a second pass changes nothing, so
// normalising text twice (as reading and splitting a source do) cannot move a
// quote's match.
func FuzzStripInvisible(f *testing.F) {
	f.Add("score\U000E0049\U000E0047 of 0.82")
	f.Add("\u202Etxet\u202C \u2066a\u2069 \uFEFFb\u200Bc\u2060d")
	f.Add("\u0645\u06CC\u200C\u062E pass\u200Dword \U0001F468\u200D\U0001F469")
	f.Add("\u0645\u200C\u200C\u0646 \u200D")
	f.Add("a\xffb\xe2\x80")
	f.Add("")

	f.Fuzz(func(t *testing.T, s string) {
		got := StripInvisible(s)
		if strings.ContainsAny(got, "\u200B\u2060\uFEFF\u202A\u202B\u202C\u202D\u202E\u2066\u2067\u2068\u2069") {
			t.Errorf("StripInvisible(%+q) = %+q kept a character it always removes", s, got)
		}
		for _, r := range got {
			if r >= 0xE0000 && r <= 0xE007F {
				t.Errorf("StripInvisible(%+q) = %+q kept a tag character", s, got)
			}
		}
		if utf8.RuneCountInString(got) > utf8.RuneCountInString(s) {
			t.Errorf("StripInvisible(%+q) added characters", s)
		}
		if again := StripInvisible(got); again != got {
			t.Errorf("StripInvisible is not idempotent on %+q: %+q then %+q", s, got, again)
		}
	})
}
