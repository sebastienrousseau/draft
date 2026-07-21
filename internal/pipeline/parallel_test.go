// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebastienrousseau/draft/internal/config"
	"github.com/sebastienrousseau/draft/internal/engine"
)

const multiSectionSource = `Abstract
The system reached a score of 0.82 on the test set. It used 5x fewer samples than before.

Introduction
Background about the approach and prior work leading up to this method here.

Results
The experiments show consistent gains across the evaluated configurations here.`

func writeMultiSectionSource(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "paper.txt")
	if err := os.WriteFile(path, []byte(multiSectionSource), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParallelExtraction(t *testing.T) {
	cfg := testConfig(t)
	cfg.ExtractConcurrency = 4
	done, errText, logs := drain(t, cfg, []engine.Engine{okEngine("fake")}, Job{Sources: []string{writeMultiSectionSource(t)}})
	if errText != "" {
		t.Fatalf("parallel extraction should succeed: %s", errText)
	}
	if done.OutputPath == "" {
		t.Fatal("expected a saved draft")
	}
	if !hasLog(logs, "workers") {
		t.Errorf("expected a parallel-workers log, got %v", logs)
	}
}

func TestParallelExtractionFallbackOnSectionError(t *testing.T) {
	cfg := testConfig(t)
	cfg.ExtractConcurrency = 4
	// Primary settles on section 0, then errors on subsequent extract calls;
	// the failed sections retry through the chain onto the working fallback.
	primary := &fakeEngine{name: "flaky", writer: func(int) (string, bool) { return validArticle("."), false }, failExtractAfter: 1}
	fallback := okEngine("ollama")
	done, errText, _ := drain(t, cfg, []engine.Engine{primary, fallback}, Job{Sources: []string{writeMultiSectionSource(t)}})
	if errText != "" {
		t.Fatalf("should recover via fallback: %s", errText)
	}
	if done.OutputPath == "" {
		t.Error("expected a saved draft after fallback")
	}
}

func TestExtractConcurrency(t *testing.T) {
	r := NewRunner(config.Config{ExtractConcurrency: 4}, nil, nil)
	r.engineName = "claude"
	if got := r.extractConcurrency(); got != 4 {
		t.Errorf("session engine concurrency = %d, want 4", got)
	}
	r.engineName = "ollama"
	if got := r.extractConcurrency(); got != ollamaExtractConcurrency {
		t.Errorf("ollama concurrency = %d, want %d (capped for a shared GPU)", got, ollamaExtractConcurrency)
	}
	r.engineName = "claude"
	r.cfg.ExtractConcurrency = 1
	if got := r.extractConcurrency(); got != 1 {
		t.Errorf("concurrency of 1 should stay 1, got %d", got)
	}
	// An explicit low value is honoured even for Ollama.
	r.engineName = "ollama"
	r.cfg.ExtractConcurrency = 1
	if got := r.extractConcurrency(); got != 1 {
		t.Errorf("ollama should honour an explicit 1, got %d", got)
	}
}
