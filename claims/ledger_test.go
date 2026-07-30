// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"strings"
	"testing"
)

const ledgerSource = "The system reached a score of 0.82 on the test set. " +
	"Router-S used 5x fewer FLOPs than the dense baseline on the same corpus."

func ledgerRecords() []Record {
	return []Record{
		{Claim: "The system reached a score of 0.82", SourceQuote: "reached a score of 0.82 on the test set", Type: "metric", Strength: "demonstrated"},
		{Claim: "Router-S used 5x fewer FLOPs", SourceQuote: "used 5x fewer FLOPs than the dense baseline", Type: "result", Strength: "demonstrated"},
	}
}

// Resume reuses extraction work only if a written ledger can be read back
// intact. RenderLedger's header would otherwise be counted as a dropped record.
func TestParseLedgerRoundTrip(t *testing.T) {
	want := ledgerRecords()
	rendered := RenderLedger(want, 3)

	got, dropped := ParseLedger(rendered, ledgerSource)
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0: the rendered header must not count as a record", dropped)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d records, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("record %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// A resumed ledger is trusted because it still passes the gate, not because we
// wrote it. A record whose source has changed underneath must be dropped.
func TestParseLedgerRejectsRecordsTheSourceNoLongerSupports(t *testing.T) {
	rendered := RenderLedger(ledgerRecords(), 0)

	// The paper was edited: the FLOPs sentence is gone.
	changed := "The system reached a score of 0.82 on the test set."

	got, dropped := ParseLedger(rendered, changed)
	if len(got) != 1 {
		t.Fatalf("kept %d records, want 1", len(got))
	}
	if dropped != 1 {
		t.Errorf("dropped = %d, want 1", dropped)
	}
	if !strings.Contains(got[0].Claim, "0.82") {
		t.Errorf("the surviving record is the wrong one: %+v", got[0])
	}
}

func TestParseLedgerOnAnEmptyLedger(t *testing.T) {
	got, dropped := ParseLedger(RenderLedger(nil, 0), ledgerSource)
	if len(got) != 0 || dropped != 0 {
		t.Errorf("got %d records / %d dropped, want 0/0", len(got), dropped)
	}
}

// Parse is unchanged: a bare record stream with no header still works.
func TestParseLedgerWithoutAHeader(t *testing.T) {
	var b strings.Builder
	for _, rec := range ledgerRecords() {
		writeRecord(&b, rec)
	}
	got, dropped := ParseLedger(b.String(), ledgerSource)
	if len(got) != 2 || dropped != 0 {
		t.Errorf("got %d records / %d dropped, want 2/0", len(got), dropped)
	}
}
