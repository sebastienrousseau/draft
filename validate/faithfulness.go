// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/internal/mdspan"
	"github.com/sebastienrousseau/draft/rules"
)

// Options tunes the checks whose strictness is a policy choice rather than a
// correctness one.
type Options struct {
	// StrictNumbers makes a number that appears in the article and in no
	// claim block the save instead of merely warning.
	//
	// A fabricated number is the clearest possible sign of invention, and for
	// a tool whose promise is that every sentence is grounded, letting one
	// reach disk with a line on stderr is hard to defend. It is opt-in for
	// one release because the naive check is noisy — see ungroundedNumbers —
	// so the real false-positive rate can be measured on a corpus before it
	// becomes the default.
	StrictNumbers bool
}

// indexOutsideProtected returns the offset of the first occurrence of term in s
// that does not fall inside a protected span, or -1.
func indexOutsideProtected(s, term string, protected [][]int) int {
	for from := 0; from <= len(s)-len(term); {
		i := strings.Index(s[from:], term)
		if i < 0 {
			return -1
		}
		at := from + i
		if !mdspan.Covers(protected, at) {
			return at
		}
		from = at + 1
	}
	return -1
}

// Faithfulness cross-checks a draft against the verified claim ledger with the
// default policy. Hard errors (returned first) block a save; warnings are
// advisory.
func Faithfulness(article string, records []claims.Record) (errs, warnings []string) {
	return FaithfulnessWithOptions(article, records, Options{})
}

// FaithfulnessWithOptions is Faithfulness with an explicit policy.
func FaithfulnessWithOptions(article string, records []claims.Record, opts Options) (errs, warnings []string) {
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
	if ungrounded := ungroundedNumbers(article, records, opts.StrictNumbers); len(ungrounded) > 0 {
		if opts.StrictNumbers {
			errs = append(errs, ungrounded...)
		} else {
			warnings = append(warnings, ungrounded...)
		}
	}
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

// orderedListMarker matches the "1." that opens a Markdown ordered-list item.
// It is document structure, not a factual number, and counting it as
// ungrounded is the second-largest source of noise in this check.
var orderedListMarker = regexp.MustCompile(`(?m)^[ \t]*\d+\.[ \t]`)

// ungroundedNumbers lists numbers that appear in the article and in no claim.
//
// When blocking, two classes are excluded because they are structure or
// metadata rather than invented evidence, and including them would fail
// honest drafts:
//
//   - ordered-list markers, stripped before the scan;
//   - four-digit years, which fill a paper's prose as publication and
//     citation dates and rarely survive into a claim.
//
// Both are still reported when the result is only advisory, so nothing is
// hidden from a reader of the warnings.
func ungroundedNumbers(article string, records []claims.Record, blocking bool) []string {
	allowed := map[string]bool{}
	for _, rec := range records {
		for n := range claims.Numbers(rec.Claim) {
			allowed[n] = true
		}
		for n := range claims.Numbers(rec.SourceQuote) {
			allowed[n] = true
		}
	}
	// Stripping list markers costs a pass over the article, so only do it when
	// the answer blocks a save. In advisory mode everything is reported
	// anyway, so nothing is hidden by leaving them in.
	scan := article
	if blocking {
		scan = orderedListMarker.ReplaceAllString(article, "")
	}
	var ungrounded []string
	for n := range claims.Numbers(scan) {
		if allowed[n] {
			continue
		}
		if blocking && yearLike(n) {
			continue
		}
		ungrounded = append(ungrounded, n)
	}
	if len(ungrounded) == 0 {
		return nil
	}
	sort.Strings(ungrounded)
	return []string{"numbers not found in any claim: " + strings.Join(ungrounded, ", ")}
}

// yearLike reports a bare four-digit number in a plausible year range.
func yearLike(n string) bool {
	if len(n) != 4 {
		return false
	}
	v, err := strconv.Atoi(n)
	return err == nil && v >= 1000 && v <= 2999
}
