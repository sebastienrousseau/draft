// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"strings"
	"testing"
)

const src = `The model reached a val_bpb of 0.82 on the held-out set and used 5x fewer tokens.`

func TestVerifyRejections(t *testing.T) {
	cases := []struct {
		rec    Record
		reason string
	}{
		{Record{Claim: "", SourceQuote: "val_bpb of 0.82"}, "missing"},
		{Record{Claim: "c", SourceQuote: "short"}, "quote too short"},
		{Record{Claim: "c", SourceQuote: "this text is not in the source at all"}, "not found"},
		{Record{Claim: "c", SourceQuote: "reached a val_bpb of 0.82 on the"}, "truncated fragment"},
		{Record{Claim: "c", SourceQuote: "val_bpb of 0.82 on the held-out set", Type: "bogus"}, "invalid TYPE"},
		{Record{Claim: "c", SourceQuote: "val_bpb of 0.82 on the held-out set", Strength: "bogus"}, "invalid STRENGTH"},
		{Record{Claim: "reached 0.99 accuracy", SourceQuote: "val_bpb of 0.82 on the held-out set"}, "numbers absent"},
	}
	for _, c := range cases {
		ok, reason := Verify(c.rec, src)
		if ok {
			t.Errorf("expected rejection for %+v", c.rec)
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("reason = %q, want to contain %q", reason, c.reason)
		}
	}
}

func TestVerifyAccepts(t *testing.T) {
	ok, _ := Verify(Record{Claim: "used 5x fewer tokens", SourceQuote: "used 5x fewer tokens", Type: "result", Strength: "demonstrated"}, src)
	if !ok {
		t.Error("valid record should verify")
	}
}

func TestParseNoneAndEmpty(t *testing.T) {
	if recs, dropped := Parse("NONE", src); len(recs) != 0 || dropped != 0 {
		t.Error("NONE should yield nothing")
	}
	if recs, _ := Parse("   ", src); len(recs) != 0 {
		t.Error("blank should yield nothing")
	}
}

func TestParseMultilineFields(t *testing.T) {
	text := "CLAIM: used 5x fewer tokens\n  continued detail\nSOURCE_QUOTE: \"used 5x fewer tokens\"\nTYPE: result\nSTRENGTH: demonstrated\n---"
	recs, _ := Parse(text, src)
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if !strings.Contains(recs[0].Claim, "continued detail") {
		t.Errorf("multi-line claim not captured: %q", recs[0].Claim)
	}
}

func TestRenderLedgerEmpty(t *testing.T) {
	if got := RenderLedger(nil, 0); !strings.Contains(got, "NONE") {
		t.Errorf("empty ledger should say NONE, got %q", got)
	}
}

func TestNumbersStripsSeparators(t *testing.T) {
	nums := Numbers("1,000 and 3.5")
	if !nums["1000"] || !nums["3.5"] {
		t.Errorf("numbers not parsed: %v", nums)
	}
}

func TestRenderPromptLedgerCharCap(t *testing.T) {
	recs := []Record{
		{Claim: strings.Repeat("a", 40), SourceQuote: "used 5x fewer tokens", Type: "result", Strength: "demonstrated"},
		{Claim: strings.Repeat("b", 40), SourceQuote: "used 5x fewer tokens", Type: "result", Strength: "demonstrated"},
	}
	out := RenderPromptLedger(recs, 100, 250) // char budget fits exactly one block
	if !strings.Contains(out, "Included records: 1 of 2") {
		t.Errorf("char cap not applied:\n%s", out)
	}
}

func TestRenderLedgerWithRecords(t *testing.T) {
	recs := []Record{{Claim: "c1", SourceQuote: "q1", Type: "metric", Strength: "demonstrated"}}
	out := RenderLedger(recs, 2)
	if !strings.Contains(out, "Verified records: 1") || !strings.Contains(out, "CLAIM: c1") || !strings.Contains(out, "Dropped records") {
		t.Errorf("full ledger malformed:\n%s", out)
	}
}
