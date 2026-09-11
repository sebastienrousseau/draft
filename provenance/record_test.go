// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package provenance

import (
	"encoding/json"
	"testing"
)

func okReport() CheckReport {
	return CheckReport{
		Generator:     Generator{Name: "draft", Version: "0.0.36"},
		BodySHA256:    "abc",
		DigestMatches: true,
		Grounding:     Grounding{Claims: []string{"c1", "c2"}, Sentences: 10, Attributed: 8, SentencesWithUngroundedNumbers: 1},
	}
}

func TestRecordMapsReport(t *testing.T) {
	r := okReport()
	r.LedgerChecked, r.LedgerMatches = true, true
	r.Sources = []SourceCheck{{Path: "p.pdf", Found: true, Matches: true}}
	rec := NewVerificationRecord(r, nil)

	if rec.Kind != RecordKind {
		t.Errorf("kind = %q", rec.Kind)
	}
	if !rec.Verified {
		t.Error("a clean report should verify")
	}
	if rec.Article.SHA256 != "abc" || !rec.Article.Matches {
		t.Errorf("article = %+v", rec.Article)
	}
	if rec.Grounding.Claims != 2 || rec.Grounding.Attributed != 8 || rec.Grounding.UngroundedNumberSentences != 1 {
		t.Errorf("grounding = %+v", rec.Grounding)
	}
	if rec.Ledger == nil || !rec.Ledger.Matches {
		t.Errorf("ledger = %+v", rec.Ledger)
	}
	if len(rec.Sources) != 1 || !rec.Sources[0].Matches {
		t.Errorf("sources = %+v", rec.Sources)
	}
}

func TestRecordVerdictWithSignature(t *testing.T) {
	base := okReport()

	// A checked, valid signature keeps the verdict true (untrusted is fine).
	rec := NewVerificationRecord(base, &SignatureState{Present: true, Checked: true, Valid: true, Trusted: false})
	if !rec.Verified {
		t.Error("valid (untrusted) signature should still verify")
	}
	// A checked, invalid signature fails the verdict even with clean digests.
	rec = NewVerificationRecord(base, &SignatureState{Present: true, Checked: true, Valid: false})
	if rec.Verified {
		t.Error("an invalid signature must fail the record")
	}
	// A present-but-unchecked signature does not fail the verdict.
	rec = NewVerificationRecord(base, &SignatureState{Present: true, Checked: false})
	if !rec.Verified {
		t.Error("an unchecked signature must not fail the record")
	}
}

func TestRecordDigestMismatchFails(t *testing.T) {
	r := okReport()
	r.DigestMatches = false
	if NewVerificationRecord(r, nil).Verified {
		t.Error("a digest mismatch must fail the record")
	}
}

func TestRecordMarshalsStably(t *testing.T) {
	rec := NewVerificationRecord(okReport(), &SignatureState{Present: true, Checked: true, Valid: true, State: "Valid"})
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	// Round-trips and keeps the stable kind marker.
	var back VerificationRecord
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Kind != RecordKind || !back.Verified || back.Signature == nil || !back.Signature.Valid {
		t.Errorf("round-trip lost data: %+v", back)
	}
}
