// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sebastienrousseau/draft/rules"
)

// Verify reports whether a record is trustworthy and, when not, why.
func Verify(rec Record, source string) (bool, string) {
	quote := strings.TrimSpace(rec.SourceQuote)
	if rec.Claim == "" || quote == "" {
		return false, "missing claim or quote"
	}
	if len([]rune(quote)) < rules.MinQuoteChars {
		return false, "quote too short"
	}
	// A quote must be well-formed text before it can be compared.
	//
	// Comparison normalises with strings.ToLower, which maps every invalid
	// UTF-8 byte to U+FFFD — so two DIFFERENT invalid byte sequences normalise
	// to the same string, and a fabricated quote could match a source it does
	// not occur in. Found by FuzzParse. Requiring the quote to be valid UTF-8
	// and free of U+FFFD closes that: normalisation can only ever introduce
	// U+FFFD, never other characters, so a quote containing none of it cannot
	// be matched against mangled bytes.
	if !utf8.ValidString(quote) {
		return false, "quote is not valid UTF-8"
	}
	if strings.ContainsRune(quote, utf8.RuneError) {
		return false, "quote contains a replacement character"
	}
	if !quoteInSource(quote, source) {
		return false, "quote not found in source"
	}
	if danglingTail.MatchString(quote) {
		return false, "quote is a truncated fragment"
	}
	recType := strings.ToLower(strings.TrimSpace(rec.Type))
	if recType != "" && !rules.ClaimTypes[recType] {
		return false, "invalid TYPE '" + recType + "'"
	}
	strength := strings.ToLower(strings.TrimSpace(rec.Strength))
	if strength != "" && !rules.ClaimStrengths[strength] {
		return false, "invalid STRENGTH '" + strength + "'"
	}

	quoteNums := Numbers(quote)
	var missing []string
	for n := range Numbers(rec.Claim) {
		if !quoteNums[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return false, "claim numbers absent from quote: " + strings.Join(missing, ", ")
	}
	return true, ""
}

// Numbers returns the distinct numeric tokens in s, with thousands separators
// stripped so "1,000" and "1000" compare equal.
func Numbers(s string) map[string]bool {
	out := map[string]bool{}
	for _, n := range numberPat.FindAllString(s, -1) {
		out[strings.ReplaceAll(n, ",", "")] = true
	}
	return out
}

func quoteInSource(quote, source string) bool {
	normQuote := Normalise(quote)
	normSource := Normalise(source)
	if strings.Contains(normSource, normQuote) {
		return true
	}
	// If the model dropped hyphens or dashes present in the source (e.g.
	// "polynomial-time" vs "polynomialtime", or "score — up" vs "score up"),
	// check if the quote matches the source with dashes collapsed.
	// A quote can never invent dashes: if the quote itself contains dashes
	// that did not match above, it is ungrounded.
	if !containsDash(quote) && containsDash(source) {
		return strings.Contains(stripDashes(normSource), stripDashes(normQuote))
	}
	return false
}

func containsDash(s string) bool {
	return strings.ContainsAny(s, "-–—‐‑")
}

func isDash(r rune) bool {
	return strings.ContainsRune("-–—‐‑", r)
}

// stripDashes drops the dashes a model may leave out of a quote, except a
// dash touching a digit. That one is part of a number: dropping it turns the
// range "10-20" into the number 1020 and "-5" into 5, and Verify checks a
// claim's numbers against its quote. Found by FuzzParse.
func stripDashes(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i := 0; i < len(rs); i++ {
		if !isDash(rs[i]) {
			b.WriteRune(rs[i])
			continue
		}
		j := i
		for j < len(rs) && isDash(rs[j]) {
			j++
		}
		if (i > 0 && unicode.IsDigit(rs[i-1])) || (j < len(rs) && unicode.IsDigit(rs[j])) {
			b.WriteString(string(rs[i:j]))
		}
		i = j - 1
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Normalise is the comparison form of source text and quotes. Two strings
// normalise equal when they are the same words in the same order, whatever
// the model or the PDF extractor did to the characters between them.
//
// Every rule here was paid for by a dropped claim. Measured over 3,217
// extraction blocks on 2026-09-06, "quote not found in source" was 552 of
// 953 drops, and the quotes were the source's words with a literal "\n"
// where a line broke, a ligature the PDF rendered as one glyph, or a
// non-breaking space. None of those is a different fact. A quote that differs
// by a word still fails.
func Normalise(s string) string {
	return strings.ToLower(canonical(s))
}

// canonical is Normalise without the case fold, so a caller can slice the
// source's own text at a position found in the folded form.
//
// Each rewrite is guarded by a cheap scan, because the overwhelming majority
// of quotes and source spans are plain ASCII with none of the characters these
// rules touch. Running a Replacer or a regex unconditionally allocates a new
// string every time even when nothing matches; skipping them keeps the common
// path to a single Fields/Join.
func canonical(s string) string {
	if strings.IndexByte(s, '\\') >= 0 {
		s = escapes.Replace(s)
	}
	if strings.IndexByte(s, '\n') >= 0 && strings.IndexByte(s, '-') >= 0 {
		s = joinHyphenBreaks(s)
	}
	if needsGlyphFold(s) {
		s = glyphs.Replace(s)
	}
	return strings.Join(strings.Fields(s), " ")
}

// joinHyphenBreaks joins a word the extractor split at a line end, but not
// digits either side of one: "10-\n20" is a range that wrapped, and joining
// it would ground the number 1020. Found by FuzzParse.
func joinHyphenBreaks(s string) string {
	var b strings.Builder
	last := 0
	for _, m := range hyphenBreak.FindAllStringIndex(s, -1) {
		if (m[0] > 0 && isASCIIDigit(s[m[0]-1])) || (m[1] < len(s) && isASCIIDigit(s[m[1]])) {
			continue
		}
		b.WriteString(s[last:m[0]])
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

func isASCIIDigit(c byte) bool { return '0' <= c && c <= '9' }

// needsGlyphFold reports whether s contains any non-ASCII byte that the glyph
// replacer rewrites (ligatures, curly quotes, invisible spaces).
func needsGlyphFold(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return true
		}
	}
	return false
}

var (
	// escapes turns the two-character sequences a model writes for a line
	// break or tab into the whitespace they stood for.
	escapes = strings.NewReplacer(`\n`, " ", `\r`, " ", `\t`, " ")
	// hyphenBreak joins a word the extractor split at a line end.
	hyphenBreak = regexp.MustCompile(`-\s*\n\s*`)
	// glyphs maps typographic variants onto the characters a model types,
	// and removes the invisible characters that separate the same word in
	// two renderings.
	glyphs = strings.NewReplacer(
		"“", `"`, "”", `"`, "‘", "'", "’", "'",
		"ﬁ", "fi", "ﬂ", "fl", "ﬀ", "ff", "ﬃ", "ffi", "ﬄ", "ffl",
		"\u00a0", " ", "\u00ad", "", "\u200b", "", "\ufeff", "",
	)
)

// maxRepairExtension bounds how far a truncated quote may be extended to
// reach the end of its sentence. Past this a "sentence" is a paragraph, and
// the claim was never about it.
const maxRepairExtension = 240

// repair extends a quote that is the source's own words but was cut short:
// a fragment ending on a conjunction, or a span shorter than a claim may
// cite. The extension is taken from the source, never from the model, so a
// repaired quote is verbatim by construction; Verify then judges it like any
// other. A quote the source does not contain word for word is left alone.
func repair(rec Record, source string) Record {
	quote := strings.TrimSpace(rec.SourceQuote)
	if quote == "" || len(quote) > maxRepairExtension {
		return rec
	}
	tooShort := utf8.RuneCountInString(quote) < rules.MinQuoteChars
	if !tooShort && !danglingTail.MatchString(quote) {
		return rec
	}
	// Search the canonicalised source in a case fold; slice the same
	// canonicalised source, un-folded, so the result is the source's text.
	flat := canonical(source)
	low := strings.ToLower(flat)
	if len(low) != len(flat) {
		return rec // a case fold that moves bytes cannot be sliced safely
	}
	needle := strings.ToLower(canonical(quote))
	start := strings.Index(low, needle)
	if start < 0 {
		return rec
	}
	end := start + len(needle)
	limit := end + maxRepairExtension
	if limit > len(flat) {
		limit = len(flat)
	}
	for i := end; i < limit; i++ {
		c := flat[i]
		if (c == '.' || c == '!' || c == '?') && (i+1 == len(flat) || flat[i+1] == ' ') {
			rec.SourceQuote = flat[start : i+1]
			return rec
		}
	}
	return rec
}
