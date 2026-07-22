// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package validate enforces the house article rules before a draft is saved:
// required structure, length, banned vocabulary, emoji, truncation, and
// faithfulness to the verified claim ledger.
package validate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sebastienrousseau/draft/internal/claims"
	"github.com/sebastienrousseau/draft/internal/rules"
)

var (
	h2Pat          = regexp.MustCompile(`(?m)^##\s+.+$`)
	tagPat         = regexp.MustCompile(`<[^>]+>`)
	mdNoisePat     = regexp.MustCompile("[#>*`_]+")
	wordTokenPat   = regexp.MustCompile(`[a-z][a-z-]{4,}`)
	paraWordPat    = regexp.MustCompile(`[a-z0-9]+`)
	blankLinePat   = regexp.MustCompile(`\n\s*\n`)
	bannedWordRe   = compileWordBoundary(rules.BannedWords)
	placeholderPat = regexp.MustCompile(`(?mi)^(#{1,6}[ \t]*(\.{2,}|…)[ \t]*$|\*\*[ \t]*(\.{2,}|…)[ \t]*\*\*|\*\*opening thesis paragraph)`)
)

// Duplicate-detection tuning.
const (
	duplicateThreshold = 0.8
	shingleK           = 4
	duplicateMinWords  = 20
)

// Errors returns the hard rule violations that must block a save. An empty
// slice means the draft is publishable.
func Errors(md string) []string {
	var errs []string
	if !strings.HasPrefix(md, rules.H1Prefix) {
		errs = append(errs, "body-only mode must start with a Markdown H1")
	}
	if !strings.Contains(md, rules.PostLeadAsideMarker) {
		errs = append(errs, "missing post-lead aside")
	}
	if !strings.Contains(md, rules.ExecSummaryMarker) {
		errs = append(errs, "missing Executive Summary")
	}
	if !h2Pat.MatchString(md) {
		errs = append(errs, "missing section headings")
	}
	if placeholderPat.MatchString(md) {
		errs = append(errs, "contains an unfilled skeleton placeholder (title, heading, or thesis)")
	}
	if w := WordCount(md); w < rules.MinWords {
		errs = append(errs, fmt.Sprintf("article is %d words; minimum is %d", w, rules.MinWords))
	}
	if ContainsEmoji(md) {
		errs = append(errs, "contains emoji")
	}
	lowered := strings.ToLower(md)
	if hits := bannedWordRe.FindAllString(lowered, -1); len(hits) > 0 {
		errs = append(errs, "contains banned words: "+strings.Join(dedupeStrings(hits), ", "))
	}
	var phrases []string
	for _, p := range rules.BannedPhrases {
		if strings.Contains(lowered, p) {
			phrases = append(phrases, p)
		}
	}
	if len(phrases) > 0 {
		errs = append(errs, "contains banned phrases: "+strings.Join(phrases, ", "))
	}
	return errs
}

// Faithfulness cross-checks a draft against the verified claim ledger. Hard
// errors (returned first) block a save; warnings are advisory.
func Faithfulness(article string, records []claims.Record) (errs, warnings []string) {
	low := strings.ToLower(strings.Join(strings.Fields(article), " "))
	var blob strings.Builder
	for _, rec := range records {
		blob.WriteString(rec.Claim + " " + rec.SourceQuote + " ")
	}
	claimsBlob := strings.ToLower(strings.Join(strings.Fields(blob.String()), " "))

	for _, term := range rules.MetricTerms {
		var found bool
		if strings.ContainsAny(term, " -") {
			found = strings.Contains(low, term)
		} else {
			found = wordBoundaryContains(low, term)
		}
		if found && !metricGrounded(term, claimsBlob) {
			errs = append(errs, "uses metric term '"+term+"' that appears in no claim (possible conversion)")
		}
	}

	if tail := strings.TrimRight(article, " \t\r\n"); tail != "" && !EndsSentence(tail) {
		errs = append(errs, "article appears truncated (does not end on sentence punctuation)")
	}
	errs = append(errs, duplicateParagraphs(article)...)

	warnings = append(warnings, hedgeUpgrades(article, records)...)
	warnings = append(warnings, ungroundedNumbers(article, records)...)
	return errs, warnings
}

// metricGrounded reports whether the metric term — or any equivalent surface form
// of the same metric — appears in the claims. An expansion or abbreviation of one
// metric (bpb and "bits per byte") counts as grounded; a switch to a different
// metric does not, because those live in separate groups.
func metricGrounded(term, claimsBlob string) bool {
	for _, form := range rules.MetricForms(term) {
		if strings.Contains(claimsBlob, form) {
			return true
		}
	}
	return false
}

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

// WordCount counts alphanumeric word tokens.
func WordCount(s string) int {
	return len(strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}))
}

// LooksLikeArticle reports whether s resembles a Markdown article body, used to
// decide whether a failed draft is still worth saving for manual review.
func LooksLikeArticle(s string) bool {
	return strings.HasPrefix(s, rules.H1Prefix) && h2Pat.MatchString(s)
}

func hedgeUpgrades(article string, records []claims.Record) []string {
	var hedged []map[string]bool
	for _, rec := range records {
		if rules.HedgeStrengths[strings.ToLower(strings.TrimSpace(rec.Strength))] {
			hedged = append(hedged, writerTokens(rec.Claim))
		}
	}
	if len(hedged) == 0 {
		return nil
	}
	var warnings []string
	for _, sentence := range writerSentences(article) {
		lowered := strings.ToLower(sentence)
		if !containsAny(lowered, rules.AssertiveVerbs) {
			continue
		}
		tokens := writerTokens(sentence)
		for _, claimTokens := range hedged {
			if sharedTokens(tokens, claimTokens) >= 2 {
				warnings = append(warnings, "possible hedge upgrade: \""+snippet(sentence, 90)+"\"")
				break
			}
		}
	}
	return warnings
}

func ungroundedNumbers(article string, records []claims.Record) []string {
	allowed := map[string]bool{}
	for _, rec := range records {
		for n := range claims.Numbers(rec.Claim) {
			allowed[n] = true
		}
		for n := range claims.Numbers(rec.SourceQuote) {
			allowed[n] = true
		}
	}
	var ungrounded []string
	seen := map[string]bool{}
	for n := range claims.Numbers(article) {
		if !allowed[n] && !seen[n] {
			ungrounded = append(ungrounded, n)
			seen[n] = true
		}
	}
	if len(ungrounded) == 0 {
		return nil
	}
	sort.Strings(ungrounded)
	return []string{"numbers not found in any claim: " + strings.Join(ungrounded, ", ")}
}

func duplicateParagraphs(article string) []string {
	type entry struct {
		text  string
		shing map[string]bool
	}
	var entries []entry
	for _, para := range blankLinePat.Split(article, -1) {
		para = strings.TrimSpace(para)
		if para == "" || strings.HasPrefix(strings.TrimLeft(para, " \t"), "#") {
			continue
		}
		words := paragraphWords(para)
		if len(words) < duplicateMinWords {
			continue
		}
		entries = append(entries, entry{text: strings.Join(strings.Fields(para), " "), shing: shingles(words, shingleK)})
	}

	var dups []string
	flagged := map[int]bool{}
	for j := range entries {
		if flagged[j] {
			continue
		}
		for i := 0; i < j; i++ {
			if jaccard(entries[j].shing, entries[i].shing) >= duplicateThreshold {
				dups = append(dups, snippet(entries[j].text, 70))
				flagged[j] = true
				break
			}
		}
	}
	switch {
	case len(dups) == 0:
		return nil
	case len(dups) <= 2:
		out := make([]string, 0, len(dups))
		for _, d := range dups {
			out = append(out, fmt.Sprintf("near-duplicate paragraph (>= %.0f%% overlap with an earlier one): %q...", duplicateThreshold*100, d))
		}
		return out
	default:
		return []string{fmt.Sprintf("%d paragraphs nearly duplicate earlier ones (>= %.0f%% overlap); give each section distinct content", len(dups), duplicateThreshold*100)}
	}
}

func writerSentences(text string) []string {
	plain := mdNoisePat.ReplaceAllString(tagPat.ReplaceAllString(text, "\n"), " ")
	var out []string
	for _, line := range strings.Split(plain, "\n") {
		start := 0
		for i := 0; i < len(line); i++ {
			if c := line[i]; c == '.' || c == '!' || c == '?' {
				if seg := strings.TrimSpace(line[start : i+1]); seg != "" {
					out = append(out, seg)
				}
				start = i + 1
			}
		}
		if seg := strings.TrimSpace(line[start:]); seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

func writerTokens(text string) map[string]bool {
	out := map[string]bool{}
	for _, w := range wordTokenPat.FindAllString(strings.ToLower(text), -1) {
		if rules.WriterStopwords[w] {
			continue
		}
		out[strings.TrimSuffix(w, "s")] = true
	}
	return out
}

func paragraphWords(para string) []string {
	return paraWordPat.FindAllString(strings.ToLower(tagPat.ReplaceAllString(para, " ")), -1)
}

func shingles(words []string, k int) map[string]bool {
	out := map[string]bool{}
	if len(words) < k {
		if len(words) > 0 {
			out[strings.Join(words, "\x00")] = true
		}
		return out
	}
	for i := 0; i <= len(words)-k; i++ {
		out[strings.Join(words[i:i+k], "\x00")] = true
	}
	return out
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func wordBoundaryContains(s, term string) bool {
	for idx := 0; idx <= len(s)-len(term); {
		j := strings.Index(s[idx:], term)
		if j < 0 {
			return false
		}
		pos := idx + j
		before := pos == 0 || !isWordByte(s[pos-1])
		end := pos + len(term)
		after := end >= len(s) || !isWordByte(s[end])
		if before && after {
			return true
		}
		idx = pos + 1
	}
	return false
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func compileWordBoundary(words []string) *regexp.Regexp {
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = regexp.QuoteMeta(w)
	}
	return regexp.MustCompile(`\b(?:` + strings.Join(quoted, "|") + `)\b`)
}

func containsAny(s string, terms []string) bool {
	for _, t := range terms {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}

func sharedTokens(a, b map[string]bool) int {
	n := 0
	for t := range a {
		if b[t] {
			n++
		}
	}
	return n
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
