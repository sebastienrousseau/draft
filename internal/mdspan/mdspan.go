// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package mdspan locates the regions of a Markdown document whose wording is
// not the writer's own: code, and quoted spans.
//
// The house-style machinery rewrites banned vocabulary in place, which is the
// cheap alternative to regenerating a whole article. Applied indiscriminately
// it also rewrites the inside of a quotation — turning
//
//	The paper states: "we leverage a robust seamless pipeline"
//
// into a sentence that attributes words to a source which never wrote them.
// That is the one edit this tool must never make, so the replacers and the
// banned-vocabulary check both run outside these spans.
//
// Blockquotes are deliberately NOT protected. In this house style a blockquote
// is the article's own Executive Summary, not a citation, so exempting them
// would silence the vocabulary rules on a whole section of original prose.
package mdspan

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// codePat matches fenced code (both fence styles) and inline code.
var codePat = regexp.MustCompile(
	"(?s:```.*?```)" +
		"|(?s:~~~.*?~~~)" +
		"|`[^`\n]+`")

// maxQuoteBytes bounds a quotation, and a quotation also ends at a paragraph
// break, so an unbalanced quote character cannot swallow the rest of the
// document and exempt it wholesale. The bound is a long paragraph rather than
// one line: a quotation the writer wrapped, or one longer than a sentence, is
// still somebody else's words.
const maxQuoteBytes = 2000

// closers pairs each opening quotation mark with the mark that ends it:
// straight and curly double quotes, straight and curly single quotes, and
// guillemets.
var closers = map[rune]rune{
	'"':  '"',
	'“':  '”',
	'\'': '\'',
	'‘':  '’',
	'«':  '»',
}

// delimiters is every character a protected span can open with.
const delimiters = "`~\"“'‘«"

// Protected returns the byte ranges of s that must not be rewritten, in order
// and non-overlapping.
func Protected(s string) [][]int {
	return find(s, true)
}

// Quotations returns the byte ranges of the quoted spans in s, marks
// included, in order. Quotation marks inside code do not count: a string in a
// code sample is not a claim about what a source said.
func Quotations(s string) [][]int {
	return find(s, false)
}

func find(s string, withCode bool) [][]int {
	// A draft with no code and no quotation is common enough to be worth one
	// cheap pass before the regular expression, which costs roughly 90ns per
	// byte to find nothing.
	if !strings.ContainsAny(s, delimiters) {
		return nil
	}
	var out [][]int
	prev := 0
	for _, c := range codePat.FindAllStringIndex(s, -1) {
		out = quotations(s, prev, c[0], out)
		if withCode {
			out = append(out, c)
		}
		prev = c[1]
	}
	return quotations(s, prev, len(s), out)
}

// quotations appends the quoted spans in s[lo:hi], which holds no code. A
// quotation never runs into the code that follows it, so the scan stops at
// hi rather than at the end of the document.
func quotations(s string, lo, hi int, out [][]int) [][]int {
	for i := lo; i < hi; {
		r, size := utf8.DecodeRuneInString(s[i:hi])
		if end := closeOf(s[:hi], i, r, size); end > 0 {
			out = append(out, []int{i, end})
			i = end
			continue
		}
		i += size
	}
	return out
}

// closeOf returns the end of the quotation that r opens at s[i], or -1.
func closeOf(s string, i int, r rune, size int) int {
	want, ok := opening(s, i, r, size)
	if !ok {
		return -1
	}
	from := i + size
	for j := from; j < len(s) && j-from <= maxQuoteBytes; {
		c, n := utf8.DecodeRuneInString(s[j:])
		if c == '\n' && blankLineFollows(s[j+n:]) {
			return -1
		}
		if c == want && (!single(r) || closesQuote(s, j, n)) {
			return j + n
		}
		j += n
	}
	return -1
}

// opening reports whether r at s[i] opens a quotation, and the mark that
// would close it.
func opening(s string, i int, r rune, size int) (rune, bool) {
	want, ok := closers[r]
	if !ok || (single(r) && !opensQuote(s, i, size)) {
		return 0, false
	}
	return want, true
}

// single reports a single quotation mark, the one that shares its character
// with the apostrophe.
func single(r rune) bool { return r == '\'' || r == '‘' }

// opensQuote reports whether the single quote at s[i] opens a quotation
// rather than eliding letters. An opening quote follows a space or opening
// punctuation and precedes a letter, so neither "don't" nor "'90s" opens one.
func opensQuote(s string, i, size int) bool {
	before, _ := utf8.DecodeLastRuneInString(s[:i])
	after, _ := utf8.DecodeRuneInString(s[i+size:])
	opener := i == 0 || unicode.IsSpace(before) || strings.ContainsRune("([{/\"“«-–—", before)
	return opener && unicode.IsLetter(after)
}

// closesQuote reports whether the single quote at s[j] closes a quotation. A
// closing quote ends a word and is followed by no letter or digit, so the
// apostrophe inside "don't" or "it’s" leaves the quotation open.
func closesQuote(s string, j, n int) bool {
	before, _ := utf8.DecodeLastRuneInString(s[:j])
	after, _ := utf8.DecodeRuneInString(s[j+n:])
	return !unicode.IsSpace(before) && !unicode.IsLetter(after) && !unicode.IsDigit(after)
}

// blankLineFollows reports whether s, the text after a newline, opens with a
// line holding nothing but white space: a paragraph break.
func blankLineFollows(s string) bool {
	return strings.HasPrefix(strings.TrimLeft(s, " \t\r"), "\n")
}

// OutsideProtected applies fn to each region of s that is not protected and
// returns the result with the protected regions restored verbatim.
//
// It splices spans rather than substituting sentinels: a sentinel can collide
// with real document text, and the failure mode when it does is silent
// corruption of the article.
func OutsideProtected(s string, fn func(string) string) string {
	locs := Protected(s)
	if len(locs) == 0 {
		return fn(s)
	}
	var b strings.Builder
	b.Grow(len(s))
	prev := 0
	for _, loc := range locs {
		b.WriteString(fn(s[prev:loc[0]]))
		b.WriteString(s[loc[0]:loc[1]])
		prev = loc[1]
	}
	b.WriteString(fn(s[prev:]))
	return b.String()
}

// Covers reports whether offset falls inside one of the spans, which must be
// the ordered, non-overlapping output of Protected.
//
// Filtering match offsets is cheaper than blanking the document: blanking
// copies the whole string twice — once to mutate and once to re-materialise —
// on every validation of every draft, to answer a question a comparison
// already answers.
func Covers(spans [][]int, offset int) bool {
	lo, hi := 0, len(spans)
	for lo < hi {
		mid := (lo + hi) / 2
		switch {
		case offset < spans[mid][0]:
			hi = mid
		case offset >= spans[mid][1]:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}
