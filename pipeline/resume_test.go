// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
)

// The ledger name used to be date-only, so a second paper drafted the same day
// silently overwrote the first one's ledger — losing the fact-checking artefact
// --keep-artifacts exists to preserve, and leaving nothing for --resume to key
// on.
func TestLedgersDoNotCollideAcrossJobs(t *testing.T) {
	dir := t.TempDir()
	a := Job{Sources: []string{filepath.Join(dir, "router-s.pdf")}}
	b := Job{Sources: []string{filepath.Join(dir, "dense-baseline.pdf")}}

	pathA, pathB := ledgerPathFor(dir, a), ledgerPathFor(dir, b)
	if pathA == pathB {
		t.Fatalf("two different sources produced the same ledger path: %s", pathA)
	}
	for _, p := range []string{pathA, pathB} {
		if !strings.HasSuffix(p, "-verified-claim-ledger.md") {
			t.Errorf("unexpected ledger name %q", filepath.Base(p))
		}
	}

	// A merged job must not collide with a single-source job starting from the
	// same file.
	merged := Job{Sources: []string{a.Sources[0], b.Sources[0]}}
	if ledgerPathFor(dir, merged) == pathA {
		t.Error("a merged job collided with a single-source job")
	}

	// A job with no sources still yields a usable name rather than panicking.
	if got := ledgerPathFor(dir, Job{}); got == "" {
		t.Error("a source-less job produced no ledger path")
	}
}

// resumeConfig keeps artifacts so the ledger survives the first run.
func resumeConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := testConfig(t)
	cfg.KeepArtifacts = true
	return cfg
}

// Extraction is 80-95% of a run's wall clock. A resumed run must re-pay none of
// it: zero KindExtract calls.
func TestResumeSkipsExtractionEntirely(t *testing.T) {
	cfg := resumeConfig(t)
	src := writeSource(t)
	job := Job{Sources: []string{src}}

	first := okEngine("fake")
	if _, errText, _ := drain(t, cfg, []engine.Engine{first}, job); errText != "" {
		t.Fatalf("first run failed: %s", errText)
	}
	if first.extractCalls == 0 {
		t.Fatal("fixture is wrong: the first run should have extracted")
	}

	// Second run, resuming.
	cfg.Resume = true
	second := okEngine("fake")
	_, errText, logs := drain(t, cfg, []engine.Engine{second}, job)
	if errText != "" {
		t.Fatalf("resumed run failed: %s", errText)
	}
	if second.extractCalls != 0 {
		t.Errorf("resumed run made %d extraction call(s); want 0", second.extractCalls)
	}
	var reported bool
	for _, l := range logs {
		if strings.Contains(l, "resumed") && strings.Contains(l, "claim") {
			reported = true
		}
	}
	if !reported {
		t.Errorf("the resume was not reported to the user: %v", logs)
	}
}

// --resume with nothing to resume from is a normal run, not an error.
func TestResumeWithoutALedgerFallsBackToExtraction(t *testing.T) {
	cfg := resumeConfig(t)
	cfg.Resume = true

	eng := okEngine("fake")
	_, errText, logs := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("run failed: %s", errText)
	}
	if eng.extractCalls == 0 {
		t.Error("expected a normal extraction when there is no ledger")
	}
	var noted bool
	for _, l := range logs {
		if strings.Contains(l, "no ledger to resume from") {
			noted = true
		}
	}
	if !noted {
		t.Errorf("the missing ledger was not reported: %v", logs)
	}
}

// A resumed ledger is trusted because it still passes the gate, not because we
// wrote it. If the sources no longer support any of it, resume must fall back
// to extracting rather than writing from a stale ledger.
func TestResumeRejectsALedgerTheSourcesNoLongerSupport(t *testing.T) {
	cfg := resumeConfig(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(src, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	job := Job{Sources: []string{src}}

	first := okEngine("fake")
	if _, errText, _ := drain(t, cfg, []engine.Engine{first}, job); errText != "" {
		t.Fatalf("first run failed: %s", errText)
	}

	// The paper is replaced with unrelated text.
	if err := os.WriteFile(src, []byte("An entirely different paper about unrelated matters."), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg.Resume = true
	second := okEngine("fake")
	_, errText, logs := drain(t, cfg, []engine.Engine{second}, job)
	if errText != "" {
		t.Fatalf("resumed run failed: %s", errText)
	}
	if second.extractCalls == 0 {
		t.Error("a ledger that no longer verifies must trigger a fresh extraction")
	}
	var warned bool
	for _, l := range logs {
		if strings.Contains(l, "no longer verifies") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("the stale ledger was not reported: %v", logs)
	}
}

// Without --resume, an existing ledger is ignored.
func TestWithoutResumeTheLedgerIsIgnored(t *testing.T) {
	cfg := resumeConfig(t)
	job := Job{Sources: []string{writeSource(t)}}

	if _, errText, _ := drain(t, cfg, []engine.Engine{okEngine("fake")}, job); errText != "" {
		t.Fatalf("first run failed: %s", errText)
	}

	second := okEngine("fake")
	if _, errText, _ := drain(t, cfg, []engine.Engine{second}, job); errText != "" {
		t.Fatalf("second run failed: %s", errText)
	}
	if second.extractCalls == 0 {
		t.Error("extraction was skipped without --resume")
	}
}
