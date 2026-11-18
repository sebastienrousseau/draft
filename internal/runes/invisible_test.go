// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package runes

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Text a reader never sees can still steer the model that reads it. Tag
// characters spell ASCII invisibly, and bidi controls make the displayed text
// differ from the text the model gets.
func TestStripInvisibleRemovesHiddenText(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"tag characters", "score\U000E0049\U000E0047\U000E004E\U000E004F\U000E0052\U000E0045 of 0.82", "score of 0.82"},
		{"tag range ends", "a\U000E0000b\U000E007Fc", "abc"},
		{"bidi override", "total \u202Egnirts\u202C here", "total gnirts here"},
		{"bidi embeddings", "\u202A\u202B\u202Dx\u202C", "x"},
		{"bidi isolates", "\u2066a\u2067b\u2068c\u2069", "abc"},
		{"zero-width space", "docu\u200Bment", "document"},
		{"word joiner", "a\u2060b", "ab"},
		{"byte-order mark", "\uFEFFTitle", "Title"},
		{"joiner inside an ASCII word", "pass\u200Dword and to\u200Cken", "password and token"},
		{"joiner at the edges", "\u200Dstart end\u200C", "start end"},
		{"joiner beside a space", "x \u200D\u0645 \u200C", "x \u0645 "},
		{"a run of joiners", "\u0645\u200C\u200C\u0646", "\u0645\u0646"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := StripInvisible(tc.in); got != tc.want {
				t.Errorf("StripInvisible(%+q) = %+q, want %+q", tc.in, got, tc.want)
			}
		})
	}
}

// ZWNJ and ZWJ carry meaning between letters of scripts that join, and inside
// emoji sequences. Removing them would change the source's words, and a
// quote of those words would no longer match.
func TestStripInvisibleKeepsMeaningfulJoiners(t *testing.T) {
	for _, tc := range []struct{ name, in string }{
		{"Persian ZWNJ", "\u0645\u06CC\u200C\u062E\u0648\u0627\u0647\u0645"},
		{"Devanagari ZWJ", "\u0915\u094D\u200D\u0937"},
		{"emoji family", "\U0001F468\u200D\U0001F469\u200D\U0001F467"},
		{"emoji with a variation selector", "\U0001F3F3\uFE0F\u200D\U0001F308"},
		{"plain text", "Nothing hidden here, 0.82 on the test set."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := StripInvisible(tc.in); got != tc.in {
				t.Errorf("StripInvisible(%+q) = %+q, want it unchanged", tc.in, got)
			}
		})
	}
}

// Text with nothing to strip comes back as it was, malformed bytes included.
func TestStripInvisibleLeavesCleanTextAlone(t *testing.T) {
	for _, in := range []string{strings.Repeat("plain prose ", 1000), "a\xffb\xe2\x80c"} {
		if got := StripInvisible(in); got != in {
			t.Errorf("StripInvisible(%+q) = %+q, want it unchanged", in, got)
		}
	}
}

// Removing a character must not splice the stray bytes around it into a new
// one. Found by FuzzStripInvisible: the leftover bytes on either side of a tag
// character spelt another tag character.
func TestStripInvisibleCannotSpliceStrayBytes(t *testing.T) {
	in := "\xf3\U000E0048\xa0\x81\x88"
	got := StripInvisible(in)
	if !utf8.ValidString(got) || strings.ContainsRune(got, 0xE0048) {
		t.Errorf("StripInvisible(%+q) = %+q", in, got)
	}
	if in := "a\xffb\u200Bc"; StripInvisible(in) != "a"+string(utf8.RuneError)+"bc" {
		t.Errorf("StripInvisible(%+q) = %+q, want the stray byte as U+FFFD", in, StripInvisible(in))
	}
	if !utf8.ValidString(StripInvisible("\u0645\u200C\u0646\U000E0041")) {
		t.Error("stripping produced invalid UTF-8")
	}
}
