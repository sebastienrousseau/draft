// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/claims"
)

var limitationLedger = []claims.Record{
	{Claim: "The compiler reached 580 pages per second on Apple silicon.", SourceQuote: "580 pages per second on Apple silicon", Type: "metric", Strength: "demonstrated"},
	{Claim: "The method was evaluated only on English-language papers.", SourceQuote: "evaluated only on English-language papers", Type: "limitation", Strength: "demonstrated"},
	{Claim: "Scanned documents without a text layer are not supported.", SourceQuote: "Scanned documents without a text layer are not supported", Type: "limitation", Strength: "demonstrated"},
}

// A draft that states none of the limitations the writer was given fails:
// it reports what a source shows and hides what the source says it does not.
// That is the opposite of what PaperOrchestra's refinement prompt asks for
// ("Never explicitly state a limitation"), and the reason this check exists.
func TestLimitationRetentionFailsADraftThatDropsThemAll(t *testing.T) {
	article := "The compiler reached 580 pages per second on Apple silicon, a strong result."
	errs, warnings := LimitationRetention(article, limitationLedger)
	if len(errs) != 1 || !strings.Contains(errs[0], "2 limitation") {
		t.Fatalf("errs = %q, want one error naming the 2 limitations", errs)
	}
	if len(warnings) != 2 {
		t.Errorf("warnings = %q, want one per dropped limitation", warnings)
	}
}

// Keeping some is enough to save; each one dropped is still reported.
func TestLimitationRetentionWarnsForEachDropped(t *testing.T) {
	article := "The compiler reached 580 pages per second on Apple silicon. " +
		"The method was evaluated only on English-language papers, so other languages are untested."
	errs, warnings := LimitationRetention(article, limitationLedger)
	if len(errs) != 0 {
		t.Fatalf("errs = %q, want none when a limitation is kept", errs)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "Scanned documents") {
		t.Errorf("warnings = %q, want the one dropped limitation", warnings)
	}
}

func TestLimitationRetentionWithNoLimitationsOrAllKept(t *testing.T) {
	if errs, warnings := LimitationRetention("Anything at all.", limitationLedger[:1]); errs != nil || warnings != nil {
		t.Errorf("no limitations given: errs %q warnings %q, want none", errs, warnings)
	}
	article := "The method was evaluated only on English-language papers. " +
		"Scanned documents without a text layer are not supported by it."
	if errs, warnings := LimitationRetention(article, limitationLedger); errs != nil || warnings != nil {
		t.Errorf("all kept: errs %q warnings %q, want none", errs, warnings)
	}
}

// The count is what provenance records, so it must agree with the check.
func TestLimitationCounts(t *testing.T) {
	article := "The method was evaluated only on English-language papers."
	given, kept := LimitationCounts(article, limitationLedger)
	if given != 2 || kept != 1 {
		t.Errorf("LimitationCounts = %d given, %d kept; want 2, 1", given, kept)
	}
}
