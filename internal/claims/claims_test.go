// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import "testing"

const source = `The model reached a val_bpb of 0.82 on the held-out set.
Training used 5x fewer tokens than the baseline configuration.
The authors note the improvement may not generalise to larger models.`

func TestParseVerifiesQuotes(t *testing.T) {
	// Two valid records and one whose quote is not in the source.
	text := `CLAIM: The model reached a val_bpb of 0.82
SOURCE_QUOTE: "reached a val_bpb of 0.82 on the held-out set"
TYPE: metric
STRENGTH: demonstrated
---
CLAIM: Training used 5x fewer tokens
SOURCE_QUOTE: "5x fewer tokens than the baseline"
TYPE: result
STRENGTH: demonstrated
---
CLAIM: The method triples throughput
SOURCE_QUOTE: "triples throughput on every device"
TYPE: result
STRENGTH: demonstrated
---`
	records, dropped := Parse(text, source)
	if len(records) != 2 {
		t.Fatalf("expected 2 verified records, got %d", len(records))
	}
	if dropped != 1 {
		t.Fatalf("expected 1 dropped record, got %d", dropped)
	}
}

func TestVerifyRejectsUngroundedNumber(t *testing.T) {
	rec := Record{
		Claim:       "The model reached 0.99 accuracy",
		SourceQuote: "reached a val_bpb of 0.82 on the held-out set",
		Type:        "metric",
		Strength:    "demonstrated",
	}
	if ok, reason := Verify(rec, source); ok {
		t.Fatalf("expected rejection for ungrounded number, got ok (%q)", reason)
	}
}

func TestDedupe(t *testing.T) {
	in := []Record{
		{Claim: "Same claim here"},
		{Claim: "same claim here"},
		{Claim: "Different claim"},
	}
	if got := Dedupe(in); len(got) != 2 {
		t.Fatalf("expected 2 after dedupe, got %d", len(got))
	}
}

func TestRenderPromptLedgerCap(t *testing.T) {
	recs := []Record{
		{Claim: "one", SourceQuote: "q1", Type: "result", Strength: "demonstrated"},
		{Claim: "two", SourceQuote: "q2", Type: "result", Strength: "demonstrated"},
		{Claim: "three", SourceQuote: "q3", Type: "result", Strength: "demonstrated"},
	}
	out := RenderPromptLedger(recs, 2, 100000)
	if want := "Included records: 2 of 3"; !contains(out, want) {
		t.Fatalf("expected %q in ledger, got:\n%s", want, out)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
