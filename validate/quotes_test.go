// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"strings"
	"testing"
)

// The writer can put any words in quotation marks. A quotation is the
// strongest form of attribution an article makes, so one that no verified
// source quote contains is reported, and under the strict policy blocks.
func TestFaithfulnessFlagsAnInventedQuotation(t *testing.T) {
	article := `The authors call it "a revolution in how models learn" in the paper.`
	_, warnings := Faithfulness(article, recs())
	if !hasSubstr(warnings, unsourcedQuotation) {
		t.Errorf("an invented quotation was not reported: %v", warnings)
	}
	errs, warnings := FaithfulnessWithOptions(article, recs(), Options{StrictQuotes: true})
	if !hasSubstr(errs, unsourcedQuotation) {
		t.Errorf("StrictQuotes did not block an invented quotation: %v", errs)
	}
	if hasSubstr(warnings, unsourcedQuotation) {
		t.Errorf("a blocking quotation was also reported as a warning: %v", warnings)
	}
}

func TestFaithfulnessAcceptsAGroundedQuotation(t *testing.T) {
	for _, quoted := range []string{
		`"reached 0.82 on the test set."`,
		"“Reached 0.82\non the TEST set”",
		"‘may not generalise to larger models’",
		"«may not generalise»",
		`"reached 0.82 ... the test set"`,
		"\"reached 0.82 … test set\"",
	} {
		article := "The paper says the model " + quoted + " and stops there."
		errs, warnings := FaithfulnessWithOptions(article, recs(), Options{StrictQuotes: true})
		if hasSubstr(errs, unsourcedQuotation) || hasSubstr(warnings, unsourcedQuotation) {
			t.Errorf("a grounded quotation %s was flagged: %v %v", quoted, errs, warnings)
		}
	}
}

func TestFaithfulnessQuotationEdgeCases(t *testing.T) {
	for _, tc := range []struct {
		name, article string
		flagged       bool
	}{
		// A word or two in quotation marks names a term; it attributes no
		// sentence to anyone.
		{"scare quote", `The so-called "world model" reached 0.82.`, false},
		// The claim is the extractor's paraphrase. Only the quote was found in
		// the source, so only the quote may be quoted.
		{"quotes the claim", `It says "The model reached 0.82 on the test set" plainly.`, true},
		// Each fragment of an elided quotation must come from the same quote,
		// or the ellipsis splices two sources into one sentence.
		{"spliced fragments", `It "reached 0.82 ... to larger models" overall.`, true},
		{"empty fragment", `It said "... ... ..." and stopped.`, false},
		{"code is not a quotation", "Run `echo \"a revolution in how models learn\"` now.", false},
		// The lead aside is HTML, and an attribute value is markup.
		{"attribute value", `<img alt="a chart of the three groups">`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, warnings := Faithfulness(tc.article, recs())
			if got := hasSubstr(warnings, unsourcedQuotation); got != tc.flagged {
				t.Errorf("flagged = %v, want %v: %v", got, tc.flagged, warnings)
			}
		})
	}
}

// One message per quotation, naming it, so the review gate can tell an
// introduced quotation from one already in the draft.
func TestUnsourcedQuotationsNamesEach(t *testing.T) {
	article := `"one invented quotation here" and "another invented quotation there"`
	got := unsourcedQuotations(article, recs())
	if len(got) != 2 || !strings.Contains(got[0], "one invented") || !strings.Contains(got[1], "another invented") {
		t.Errorf("unsourcedQuotations() = %v", got)
	}
}
