// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sebastienrousseau/draft/rules"
)

// EndsSentence reports whether the trailing text closes on sentence-ending
// punctuation. It decodes the final rune (not the final byte) so multibyte
// closers — smart quotes, apostrophe, ellipsis — are recognised rather than
// mistaken for a truncated fragment.
func EndsSentence(tail string) bool {
	if strings.HasSuffix(tail, "---") {
		return true
	}
	last, _ := utf8.DecodeLastRuneInString(tail)
	switch last {
	case '.', '!', '?', '"', '\'', ')', ']',
		'”', '’', '…', '»':
		return true
	}
	return false
}

// ContainsEmoji reports whether s contains a pictographic or symbol emoji.
func ContainsEmoji(s string) bool {
	for _, r := range s {
		if (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) {
			return true
		}
	}
	return false
}

// WordCount counts the words of prose a reader sees: whitespace-separated
// tokens that contain a letter or a digit, after HTML comments, tags and link
// targets are removed. "3,000", "FCA's" and "AI-driven" are one word each, and
// a lone "##" or dash is none. Splitting at every non-letter instead counted
// numbers, contractions, hyphenations and the lead aside's attributes as extra
// words, and put real drafts up to 171 words over their prose length.
func WordCount(s string) int {
	var sc markupScanner
	n, counted := 0, false
	for i := 0; i < len(s); {
		if skip := sc.markupLen(s[i:]); skip > 0 {
			counted = false
			i += skip
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case unicode.IsSpace(r):
			counted = false
		case !counted && isWordRune(r):
			n++
			counted = true
		}
		i += size
	}
	return n
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }

// markupScanner finds what a reader never sees as words: an HTML comment, an
// HTML tag, and a Markdown link's target (the "(url)" after "]"). It is a hand
// scan rather than a regexp replace because Errors runs on every draft and the
// replace made it 42% slower. Once a closer is missing from a suffix it is
// missing from every later one, so remembering that keeps unclosed input linear.
type markupScanner struct{ noComment, noTag, noLink bool }

// markupLen returns the length of the markup that starts t, or 0.
func (sc *markupScanner) markupLen(t string) int {
	switch {
	case strings.HasPrefix(t, "<!--"):
		return sc.closed(t, 4, "-->", &sc.noComment)
	case t[0] == '<':
		j := 1
		if j < len(t) && t[j] == '/' {
			j++
		}
		if j < len(t) && isASCIILetter(t[j]) {
			return sc.closed(t, j, ">", &sc.noTag)
		}
	case strings.HasPrefix(t, "]("):
		return sc.closed(t, 2, ")", &sc.noLink)
	}
	return 0
}

// closed returns the length of t up to and including the first closer at or
// after from, or 0 when there is none.
func (sc *markupScanner) closed(t string, from int, closer string, missing *bool) int {
	if *missing {
		return 0
	}
	k := strings.Index(t[from:], closer)
	if k < 0 {
		*missing = true
		return 0
	}
	return from + k + len(closer)
}

func isASCIILetter(b byte) bool { return b|0x20 >= 'a' && b|0x20 <= 'z' }

// LooksLikeArticle reports whether s resembles a Markdown article body, used to
// decide whether a failed draft is still worth saving for manual review.
func LooksLikeArticle(s string) bool {
	return strings.HasPrefix(s, rules.H1Prefix) && h2Pat.MatchString(s)
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func snippet(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}
