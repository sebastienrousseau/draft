// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/internal/mdspan"
)

// unsourcedQuotation opens the message for a quotation no verified source
// quote contains. The review gate keys on the whole message, so it names the
// quotation and nothing that an unrelated edit could change.
const unsourcedQuotation = "quotation found in no verified source quote"

// minQuotedWords is the shortest quotation that is checked. A word or two in
// quotation marks names a term ("world model") rather than attributing a
// sentence to anyone, and checking those would bury the real findings.
const minQuotedWords = 3

// ellipsis marks where a quotation leaves words out.
var ellipsis = regexp.MustCompile(`\.\.\.|\x{2026}`)

// unsourcedQuotations lists the article's quotations that occur in no
// verified claim's source quote, one message each.
//
// The writer only ever sees the ledger, so a quotation it may honestly make
// is a span of some SOURCE_QUOTE, which Verify has already found verbatim in
// its source section. The claim text is the extractor's paraphrase and does
// not count: quoting it would present a model's words as the source's.
// Comparison uses claims.Normalise, the same folding Verify applies, so case,
// line breaks and typographic glyphs do not decide the answer.
func unsourcedQuotations(article string, records []claims.Record) []string {
	spans := mdspan.Quotations(article)
	if len(spans) == 0 {
		return nil
	}
	quotes := make([]string, len(records))
	for i, rec := range records {
		quotes[i] = claims.Normalise(rec.SourceQuote)
	}
	var out []string
	for _, sp := range spans {
		inner := quotationText(article[sp[0]:sp[1]])
		if attributeValue(article, sp[0]) || len(strings.Fields(inner)) < minQuotedWords || quotedFromAny(fragments(inner), quotes) {
			continue
		}
		out = append(out, unsourcedQuotation+": \""+snippet(strings.Join(strings.Fields(inner), " "), 90)+"\"")
	}
	return out
}

// attributeValue reports a quoted span that is the value of an HTML
// attribute, such as the alt text in the lead aside: markup, not a quotation.
func attributeValue(article string, start int) bool {
	return start > 0 && article[start-1] == '='
}

// quotationText strips the opening and closing marks from a quoted span.
func quotationText(q string) string {
	_, open := utf8.DecodeRuneInString(q)
	_, closing := utf8.DecodeLastRuneInString(q)
	return q[open : len(q)-closing]
}

// fragments splits a quotation at its ellipses into the normalised runs of
// words it claims the source contains, without the punctuation a sentence
// around the quotation adds at either end.
func fragments(inner string) []string {
	var out []string
	for _, part := range ellipsis.Split(claims.Normalise(inner), -1) {
		if part = strings.Trim(part, " .,;:!?\"'"); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// quotedFromAny reports whether one source quote holds every fragment. Taking
// the fragments from two different quotes would splice two sources into one
// sentence, which is a quotation neither of them made.
func quotedFromAny(frags, quotes []string) bool {
	if len(frags) == 0 {
		return true
	}
	for _, q := range quotes {
		if containsAll(q, frags) {
			return true
		}
	}
	return false
}

func containsAll(s string, parts []string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
