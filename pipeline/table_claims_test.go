// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"testing"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/pdf"
)

// A section carrying a Markdown table (as a layout reader emits) contributes
// grounded cell claims to the ledger even when the model extracts nothing.
func TestExtractClaimsMinesTables(t *testing.T) {
	table := "Results.\n\n" +
		"| Model    |   Throughput (pages/s) |   Accuracy |\n" +
		"|----------|------------------------|------------|\n" +
		"| draft-A  |                     99 |       0.82 |\n" +
		"| baseline |                     33 |       0.71 |\n"
	sections := []pdf.Section{{Label: "a", Body: table}}

	// The model declines (NONE), so every surviving claim comes from the table.
	eng := &countingEngine{name: "fake", out: "NONE"}
	dir := t.TempDir()
	cfg := config.Config{HomeDir: dir, DraftsDir: dir}
	r := NewRunner(cfg, []engine.Engine{eng}, nil)

	records, _, err := r.extractClaims(context.Background(), Job{}, sections, dir, r.chainFor(engine.KindExtract))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 {
		t.Fatalf("got %d claims from the table, want 4:\n%+v", len(records), records)
	}
	// Every mined claim is grounded in the section it came from.
	for _, rec := range records {
		if rec.SourceQuote == "" || rec.Type != "metric" {
			t.Errorf("unexpected table claim: %+v", rec)
		}
	}
}

// Plain text with no Markdown table must not gain any table claims: the
// feature is a no-op for a plain-text reader.
func TestExtractClaimsNoTableNoExtraClaims(t *testing.T) {
	sections := []pdf.Section{{Label: "a", Body: "Plain prose with no table, just a sentence about 42 widgets."}}
	eng := &countingEngine{name: "fake", out: "NONE"}
	dir := t.TempDir()
	r := NewRunner(config.Config{HomeDir: dir, DraftsDir: dir}, []engine.Engine{eng}, nil)

	records, _, err := r.extractClaims(context.Background(), Job{}, sections, dir, r.chainFor(engine.KindExtract))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Errorf("got %d claims from plain prose, want 0:\n%+v", len(records), records)
	}
}
