// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
)

// A dry run exercises the real resolve and sectioning path but must not call a
// model — that is what makes it cost ~110 ms instead of ten minutes, and what
// makes a successful plan evidence the sources are readable.
func TestDryRunMakesNoModelCalls(t *testing.T) {
	cfg := testConfig(t)
	eng := okEngine("fake")
	r := NewRunner(cfg, []engine.Engine{eng}, nil)

	rep, err := r.DryRun(context.Background(), Job{Sources: []string{writeSource(t)}})
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if eng.extractCalls != 0 || eng.writeCalls != 0 {
		t.Errorf("a dry run called a model: %d extract, %d write", eng.extractCalls, eng.writeCalls)
	}
	if rep.SectionCount == 0 {
		t.Error("no sections reported")
	}
	if rep.EstCalls != rep.SectionCount+1 {
		t.Errorf("EstCalls = %d, want sections+1 (%d)", rep.EstCalls, rep.SectionCount+1)
	}
	if rep.OutputDir == "" {
		t.Error("no output directory reported")
	}
	for _, k := range []engine.Kind{engine.KindExtract, engine.KindWrite, engine.KindEdit} {
		if rep.Engines[k] != "fake" {
			t.Errorf("kind %v routed to %q, want fake", k, rep.Engines[k])
		}
	}
	if rep.LedgerFound {
		t.Error("reported a resumable ledger where none exists")
	}
}

// The estimate collapses to a single write call when extraction is already
// paid for, which is the number that decides whether to pass --resume.
func TestDryRunReportsAResumableLedger(t *testing.T) {
	cfg := testConfig(t)
	cfg.KeepArtifacts = true
	job := Job{Sources: []string{writeSource(t)}}

	if _, errText, _ := drain(t, cfg, []engine.Engine{okEngine("fake")}, job); errText != "" {
		t.Fatalf("seed run failed: %s", errText)
	}

	r := NewRunner(cfg, []engine.Engine{okEngine("fake")}, nil)
	rep, err := r.DryRun(context.Background(), job)
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if !rep.LedgerFound {
		t.Fatal("an existing ledger was not detected")
	}
	if rep.EstCalls != 1 {
		t.Errorf("EstCalls = %d, want 1 when extraction is resumable", rep.EstCalls)
	}
}

func TestDryRunSurfacesUnusableSources(t *testing.T) {
	cfg := testConfig(t)
	r := NewRunner(cfg, []engine.Engine{okEngine("fake")}, nil)

	if _, err := r.DryRun(context.Background(), Job{}); err == nil {
		t.Error("expected an error for a job with no sources")
	}

	bad := filepath.Join(t.TempDir(), "notes.xyz")
	if err := os.WriteFile(bad, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DryRun(context.Background(), Job{Sources: []string{bad}}); err == nil {
		t.Error("expected an error for an unreadable source")
	}

	empty := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DryRun(context.Background(), Job{Sources: []string{empty}}); err == nil {
		t.Error("expected an error for a readable source with no text")
	}
}

// A Runner is reused across a queue, so each job supplies its own channel.
func TestSetEventsRedirectsTheNextRun(t *testing.T) {
	r := NewRunner(config.Config{}, []engine.Engine{okEngine("fake")}, nil)
	r.done = context.Background().Done()

	first := make(chan Event, 4)
	r.SetEvents(first)
	r.log("to the first channel")

	second := make(chan Event, 4)
	r.SetEvents(second)
	r.log("to the second channel")

	if len(first) != 1 || len(second) != 1 {
		t.Errorf("events went to the wrong channel: first=%d second=%d", len(first), len(second))
	}
}

func TestEngineForReportsThePerKindBackend(t *testing.T) {
	r := NewRoutedRunner(config.Config{
		Engine: config.EngineOllama, WriteEngine: config.EngineOllama,
	}, nil)
	if got := r.EngineFor(engine.KindWrite); got != "ollama" {
		t.Errorf("EngineFor(KindWrite) = %q, want ollama", got)
	}
	// An exhausted chain reports nothing rather than panicking.
	empty := NewRunner(config.Config{}, nil, nil)
	if got := empty.EngineFor(engine.KindWrite); got != "" {
		t.Errorf("EngineFor on an empty chain = %q, want empty", got)
	}
}
