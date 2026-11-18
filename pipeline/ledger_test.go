// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/pdf"
)

var oneSection = []pdf.Section{{Label: "paper.pdf", Body: "The system reached a score of 0.82 on the test set."}}

var keptRecord = claims.Record{
	Claim: "The system reached a score of 0.82", SourceQuote: "reached a score of 0.82 on the test set",
	Type: "metric", Strength: "demonstrated",
}

// Two runs of the same paper on the same day used to share one ledger path,
// so the first to finish deleted the ledger the other was about to write its
// article from, and its provenance would name a file that was gone.
func TestConcurrentRunsKeepSeparateLedgers(t *testing.T) {
	out := t.TempDir()
	job := Job{Sources: []string{"paper.pdf"}}
	newRunner := func() *Runner {
		return NewRunner(testConfig(t), []engine.Engine{okEngine("fake")}, make(chan Event, 512))
	}
	a, b := newRunner(), newRunner()
	for _, r := range []*Runner{a, b} {
		if _, _, err := r.resumeOrExtract(context.Background(), job, oneSection, out, r.chainFor(engine.KindExtract)); err != nil {
			t.Fatal(err)
		}
	}
	if a.ledgerPath == b.ledgerPath {
		t.Fatalf("two runs share one ledger path: %s", a.ledgerPath)
	}
	a.cleanupArtifacts()
	if _, err := os.Stat(b.ledgerPath); err != nil {
		t.Errorf("one run's cleanup removed the other run's ledger: %v", err)
	}
	if _, err := os.Stat(a.ledgerPath); !os.IsNotExist(err) {
		t.Errorf("a run did not clean up its own ledger: %v", err)
	}
}

// A reader racing the write, such as a concurrent --resume, must see the old
// ledger or the new one, never a truncated file. A rename puts a new file in
// place; an in-place write keeps the same one and truncates it first.
func TestSaveLedgerReplacesRatherThanTruncates(t *testing.T) {
	if runtime.GOOS == "windows" {
		// os.Stat on Windows reads a file's identity lazily, by path, when
		// SameFile asks, so both stats name whatever is at the path now
		// and the comparison cannot see the replacement.
		t.Skip("os.SameFile cannot compare a replaced path on Windows")
	}
	r := NewRunner(testConfig(t), []engine.Engine{okEngine("fake")}, make(chan Event, 512))
	path := filepath.Join(t.TempDir(), "ledger.md")
	if err := os.WriteFile(path, []byte("earlier ledger"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	r.saveLedger(path, []claims.Record{keptRecord}, 0)
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Error("the ledger was rewritten in place, so a reader can observe it half-written")
	}
	assertNoLedgerTemps(t, filepath.Dir(path))
}

// With a ledger per run, resume has to choose. It takes the most recent one
// for these sources, which is the attempt the user is retrying, and still
// re-verifies it.
func TestResumePicksTheNewestLedgerForTheSources(t *testing.T) {
	r, job, out, _ := resumeRunner(t, okEngine("fake"))
	older := filepath.Join(out, ledgerName(job, "aaaaaaaa"))
	newer := filepath.Join(out, ledgerName(job, "bbbbbbbb"))
	other := filepath.Join(out, ledgerName(Job{Sources: []string{"unrelated.pdf"}}, "cccccccc"))
	writeLedger(t, older, claims.Record{Claim: "Router-S was cheaper", SourceQuote: "Router-S used 5x fewer FLOPs", Strength: "demonstrated"})
	writeLedger(t, newer, keptRecord)
	writeLedger(t, other, claims.Record{Claim: "Router-S was cheaper", SourceQuote: "Router-S used 5x fewer FLOPs", Strength: "demonstrated"})
	past := time.Now().Add(-time.Hour)
	for _, p := range []string{older, other} {
		if err := os.Chtimes(p, past, past); err != nil {
			t.Fatal(err)
		}
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(other, future, future); err != nil {
		t.Fatal(err)
	}

	records, _, err := r.resumeOrExtract(context.Background(), job, mergedSections, out, r.chainFor(engine.KindExtract))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].SourceQuote != keptRecord.SourceQuote {
		t.Fatalf("resumed %+v, want only the newest ledger's record", records)
	}
	if r.ledgerPath == newer || r.ledgerPath == older {
		t.Error("a resumed run must write its own ledger, not rewrite the one it read")
	}
}

// A ledger written before ledgers carried a run token is still resumable.
func TestResumeReadsALegacyLedgerName(t *testing.T) {
	eng := okEngine("fake")
	r, job, out, _ := resumeRunner(t, eng)
	writeLedger(t, filepath.Join(out, ledgerName(job, "")), keptRecord)

	records, _, err := r.resumeOrExtract(context.Background(), job, mergedSections, out, r.chainFor(engine.KindExtract))
	if err != nil || len(records) != 1 {
		t.Fatalf("records=%d err=%v; want the legacy ledger resumed", len(records), err)
	}
	if eng.extractCalls != 0 {
		t.Errorf("a legacy ledger was not found (%d extraction calls)", eng.extractCalls)
	}
}

// Once the resumed run succeeds, the ledger it consumed has served its
// purpose; without --keep-artifacts the day folder holds only articles.
func TestCleanupRemovesTheResumedLedger(t *testing.T) {
	r, job, out, _ := resumeRunner(t, okEngine("fake"))
	r.cfg.KeepArtifacts = false
	consumed := filepath.Join(out, ledgerName(job, "aaaaaaaa"))
	writeLedger(t, consumed, keptRecord)
	if _, _, err := r.resumeOrExtract(context.Background(), job, mergedSections, out, r.chainFor(engine.KindExtract)); err != nil {
		t.Fatal(err)
	}
	r.cleanupArtifacts()
	for _, p := range []string{consumed, r.ledgerPath} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s survived cleanup: %v", filepath.Base(p), err)
		}
	}
}

// Only names this package writes are candidates: a stray file that merely
// shares the prefix is not a ledger for these sources.
func TestResumableLedgerIgnoresLookalikes(t *testing.T) {
	out := t.TempDir()
	job := Job{Sources: []string{"paper.pdf"}}
	day := time.Now().Format("2006-01-02")
	for _, name := range []string{
		day + "-paper-two" + LedgerSuffix,        // a different paper's stem
		day + "-paper.NOTHEX!!" + LedgerSuffix,   // not a run token
		day + "-paper.abcd" + LedgerSuffix + "x", // not a ledger
		day + "-paper." + LedgerSuffix,           // an empty token
		day + "-paper.ABCD" + LedgerSuffix,       // a token this package never draws
	} {
		if err := os.WriteFile(filepath.Join(out, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(out, ledgerName(job, "deadbeef")), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, ok := resumableLedger(out, job); ok {
		t.Errorf("resumableLedger picked %s", filepath.Base(got))
	}
	if _, ok := resumableLedger(filepath.Join(out, "missing"), job); ok {
		t.Error("a missing folder yielded a ledger")
	}
}

// A ledger that cannot be written is not reported as saved, and no ledger
// path is recorded for provenance or cleanup.
func TestSaveLedgerReportsAFailedWrite(t *testing.T) {
	r := NewRunner(testConfig(t), []engine.Engine{okEngine("fake")}, make(chan Event, 512))
	r.saveLedger(filepath.Join(t.TempDir(), "missing", "ledger.md"), []claims.Record{keptRecord}, 0)
	if r.ledgerPath != "" {
		t.Errorf("ledgerPath = %q after a failed write", r.ledgerPath)
	}
}

func assertNoLedgerTemps(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "ledger.md" {
			t.Errorf("stray file left behind: %s", e.Name())
		}
	}
}
