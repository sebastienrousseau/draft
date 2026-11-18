// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import "testing"

// Two sections, as two merged papers or two headings of one would arrive.
var resumeSections = []string{
	"The system reached a score of 0.82 on the test set.",
	"Router-S used 5x fewer FLOPs than the dense baseline.",
}

// A quote running from the end of one section into the start of the next
// occurs in the joined corpus, because normalisation folds the separator
// into a space, but in no source. Resume must not accept what a fresh
// extraction, which verifies per section, never could.
func TestParseLedgerSectionsRejectsAQuoteSpanningSections(t *testing.T) {
	spanning := Record{
		Claim:       "The test set result preceded Router-S",
		SourceQuote: "on the test set. Router-S used 5x fewer FLOPs",
		Strength:    "demonstrated",
	}
	ledger := RenderLedger([]Record{spanning}, 0)

	// The premise: against the joined corpus the forged span verifies.
	joined := resumeSections[0] + "\n\n" + resumeSections[1]
	if got, _ := ParseLedger(ledger, joined); len(got) != 1 {
		t.Fatalf("premise broken: the span should verify against the joined corpus")
	}

	got, dropped := ParseLedgerSections(ledger, resumeSections)
	if len(got) != 0 || dropped != 1 {
		t.Errorf("a quote spanning two sections was resumed: got %d, dropped %d", len(got), dropped)
	}
}

// Each record is kept when it verifies in any one section, whichever it is,
// and records are returned in ledger order.
func TestParseLedgerSectionsKeepsRecordsFromEachSection(t *testing.T) {
	recs := []Record{
		{Claim: "Router-S used 5x fewer FLOPs", SourceQuote: "used 5x fewer FLOPs than the dense baseline", Type: "result", Strength: "demonstrated"},
		{Claim: "The system reached a score of 0.82", SourceQuote: "reached a score of 0.82 on the test set", Type: "metric", Strength: "demonstrated"},
		{Claim: "Nothing supports this", SourceQuote: "a sentence in neither section", Strength: "demonstrated"},
	}
	got, dropped := ParseLedgerSections(RenderLedger(recs, 0), resumeSections)
	if len(got) != 2 || dropped != 1 {
		t.Fatalf("got %d records, dropped %d; want 2 and 1", len(got), dropped)
	}
	if got[0] != recs[0] || got[1] != recs[1] {
		t.Errorf("records = %+v, want the first two in ledger order", got)
	}
}

// With no sections there is nothing a record could be grounded in.
func TestParseLedgerSectionsWithNoSectionsDropsEverything(t *testing.T) {
	got, dropped := ParseLedgerSections(RenderLedger(ledgerRecords(), 0), nil)
	if len(got) != 0 || dropped != 2 {
		t.Errorf("got %d records, dropped %d; want 0 and 2", len(got), dropped)
	}
}
