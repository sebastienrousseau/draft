// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package runes

import "testing"

// Model output and provider errors are printed to the user's terminal. An
// escape sequence there can retitle the window, clear the screen, rewrite
// earlier lines or, on some terminals, write to the clipboard.
func TestTerminalSafeRemovesControls(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"CSI clear screen", "a\x1b[2Jb", "a[2Jb"},
		{"OSC title with BEL", "\x1b]0;pwned\x07done", "]0;pwneddone"},
		{"OSC 52 clipboard", "\x1b]52;c;ZWNobw==\x1b\\ok", "]52;c;ZWNobw==\\ok"},
		{"carriage return overwrite", "safe\rEVIL", "safeEVIL"},
		{"backspace", "abc\b\b\bxyz", "abcxyz"},
		{"DEL", "a\x7fb", "ab"},
		{"C1 CSI", "a\u009b2Jb", "a2Jb"},
		{"C1 OSC and ST", "\u009d0;t\u009cx", "0;tx"},
		{"NUL and BEL", "a\x00b\x07c", "abc"},
		{"vertical tab and form feed", "a\x0bb\x0cc", "abc"},
		{"stray 8-bit CSI byte", "a\x9b2Jb", "a\xef\xbf\xbd2Jb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := TerminalSafe(tc.in); got != tc.want {
				t.Errorf("TerminalSafe(%+q) = %+q, want %+q", tc.in, got, tc.want)
			}
		})
	}
}

// Line breaks and tabs are how a log line and a streamed article are laid
// out, and printable text in any script is the content itself.
func TestTerminalSafeKeepsText(t *testing.T) {
	for _, in := range []string{
		"line one\nline two\tindented",
		"Gr\u00FC\u00DFe, \u4E16\u754C, \u0645\u0631\u062D\u0628\u0627, \U0001F600",
		"",
	} {
		if got := TerminalSafe(in); got != in {
			t.Errorf("TerminalSafe(%+q) = %+q, want it unchanged", in, got)
		}
	}
}
