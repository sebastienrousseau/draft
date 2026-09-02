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
)

// protectedPat matches, in order: fenced code (both fence styles), inline code,
// and a straight- or smart-quoted span. Quoted spans are bounded to one line
// and to a sane length so an unbalanced quote character cannot swallow the rest
// of the document and exempt it wholesale.
var protectedPat = regexp.MustCompile(
	"(?s:```.*?```)" +
		"|(?s:~~~.*?~~~)" +
		"|`[^`\n]+`" +
		`|"[^"\n]{0,400}"` +
		"|“[^”\n]{0,400}”")

// Protected returns the byte ranges of s that must not be rewritten, in order
// and non-overlapping.
func Protected(s string) [][]int { return protectedPat.FindAllStringIndex(s, -1) }

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

// BlankProtected returns s with every protected region replaced by spaces,
// preserving both byte length and line structure so a scan over the result
// reports offsets and line numbers that still refer to the original.
func BlankProtected(s string) string {
	locs := Protected(s)
	if len(locs) == 0 {
		return s
	}
	b := []byte(s)
	for _, loc := range locs {
		for i := loc[0]; i < loc[1]; i++ {
			if b[i] != '\n' {
				b[i] = ' '
			}
		}
	}
	return string(b)
}
