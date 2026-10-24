// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"regexp"
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
	if strings.ContainsAny(s, "<]") {
		s = markupPat.ReplaceAllString(s, " ")
	}
	n := 0
	for _, f := range strings.Fields(s) {
		if strings.IndexFunc(f, isWordRune) >= 0 {
			n++
		}
	}
	return n
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }

// markupPat matches what a reader never sees as words: an HTML comment, an
// HTML tag, and a Markdown link's target (the "(url)" after "]").
var markupPat = regexp.MustCompile(`(?s)<!--.*?-->|</?[A-Za-z][^>]*>|\]\([^)]*\)`)

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
