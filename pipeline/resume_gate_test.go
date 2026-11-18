// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/pdf"
)

// Two sections as a --merge job of two papers presents them.
var mergedSections = []pdf.Section{
	{Label: "a.pdf", Body: "The system reached a score of 0.82 on the test set."},
	{Label: "b.pdf", Body: "Router-S used 5x fewer FLOPs than the dense baseline."},
}

// resumeRunner returns a resuming runner and a path its ledger is read from:
// the name a ledger had before it carried a run token, which resume still
// finds.
func resumeRunner(t *testing.T, engines ...engine.Engine) (*Runner, Job, string, string) {
	t.Helper()
	cfg := resumeConfig(t)
	cfg.Resume = true
	r := NewRunner(cfg, engines, make(chan Event, 512))
	job := Job{Sources: []string{"a.pdf", "b.pdf"}}
	out := t.TempDir()
	return r, job, out, filepath.Join(out, ledgerName(job, ""))
}

func writeLedger(t *testing.T, path string, recs ...claims.Record) {
	t.Helper()
	if err := os.WriteFile(path, []byte(claims.RenderLedger(recs, 0)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A ledger quote that runs from the end of one merged paper into the start
// of the next is in neither paper. Resume used to verify against the papers
// joined together, where it is found, and would hand it to the writer.
func TestResumeRejectsQuoteSpanningSections(t *testing.T) {
	eng := okEngine("fake")
	r, job, out, path := resumeRunner(t, eng)
	writeLedger(t, path, claims.Record{
		Claim:       "The test set result came before Router-S",
		SourceQuote: "on the test set. Router-S used 5x fewer FLOPs",
		Strength:    "demonstrated",
	})

	records, _, err := r.resumeOrExtract(context.Background(), job, mergedSections, out, r.chainFor(engine.KindExtract))
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range records {
		if strings.Contains(rec.SourceQuote, "Router-S") {
			t.Errorf("a quote spanning two sources was resumed: %+v", rec)
		}
	}
	if eng.extractCalls == 0 {
		t.Error("with nothing left to resume, the run must extract afresh")
	}
}

// The second gate is part of the gate. A resumed ledger that skipped it
// would let --resume launder a claim the model had judged unsupported.
func TestResumeRunsSecondGate(t *testing.T) {
	eng := okEngine("fake")
	eng.editResponse = "UNSUPPORTED"
	r, job, out, path := resumeRunner(t, eng)
	r.cfg.SecondGate = true
	writeLedger(t, path, claims.Record{
		Claim:       "The system reached a score of 0.82",
		SourceQuote: "reached a score of 0.82 on the test set",
		Type:        "metric",
		Strength:    "demonstrated",
	})

	records, _, err := r.resumeOrExtract(context.Background(), job, mergedSections, out, r.chainFor(engine.KindExtract))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Errorf("the second gate did not run on resume: kept %+v", records)
	}
	if eng.extractCalls != 0 {
		t.Errorf("a ledger that verified must not be re-extracted (%d calls)", eng.extractCalls)
	}
}

// Provenance names the ledger the article was written from. A resumed run
// used to leave the digest empty, and a resume that dropped records would
// otherwise leave a file on disk that is not the ledger used.
func TestResumeRecordsTheLedgerItUsed(t *testing.T) {
	r, job, out, path := resumeRunner(t, okEngine("fake"))
	kept := claims.Record{Claim: "The system reached a score of 0.82", SourceQuote: "reached a score of 0.82 on the test set", Type: "metric", Strength: "demonstrated"}
	gone := claims.Record{Claim: "A claim no source makes", SourceQuote: "a sentence in neither paper", Strength: "demonstrated"}
	writeLedger(t, path, kept, gone)

	records, dropped, err := r.resumeOrExtract(context.Background(), job, mergedSections, out, r.chainFor(engine.KindExtract))
	if err != nil || len(records) != 1 || dropped != 1 {
		t.Fatalf("records=%d dropped=%d err=%v; want 1, 1, nil", len(records), dropped, err)
	}
	onDisk, err := os.ReadFile(r.ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if r.ledgerDigest == "" || r.ledgerDigest != sha256Hex(string(onDisk)) {
		t.Errorf("ledger digest %q does not name the ledger on disk", r.ledgerDigest)
	}
	if strings.Contains(string(onDisk), gone.SourceQuote) {
		t.Error("the saved ledger still holds a record the resume dropped")
	}
	if r.ledgerPath == path || r.resumedFrom != path {
		t.Errorf("ledgerPath = %q, resumedFrom = %q; want a new ledger written from %q", r.ledgerPath, r.resumedFrom, filepath.Base(path))
	}
}
