// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package runes

import (
	"strings"
	"unicode/utf8"
)

// TerminalSafe removes the control characters from text bound for a
// terminal: C0 controls other than newline and tab, DEL, and the C1 controls
// U+0080-U+009F. Streamed tokens, warnings and provider errors carry text a
// model or another program chose, and an escape sequence in them could clear
// the screen, retitle the window, overwrite an earlier line with a carriage
// return or, on some terminals, set the clipboard.
//
// Removing the introducer is enough. What follows ESC or CSI is printable, so
// "[2J" without its ESC is just text, and nothing has to parse sequences that
// a chunked stream may split across calls. Bytes that are not valid UTF-8
// become U+FFFD, so a stray 0x9B cannot act as an 8-bit CSI either.
//
// It is for display only. Files and --json output keep the text as written.
func TerminalSafe(s string) string {
	if strings.IndexFunc(s, isTerminalControl) < 0 && utf8.ValidString(s) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isTerminalControl(r) {
			return -1
		}
		return r
	}, s)
}

func isTerminalControl(r rune) bool {
	if r == '\n' || r == '\t' {
		return false
	}
	return r < 0x20 || (r >= 0x7f && r <= 0x9f)
}
