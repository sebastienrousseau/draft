// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/rules"
)

// promptRecorder answers every write with out and keeps the last prompt.
type promptRecorder struct {
	out    string
	calls  atomic.Int64
	prompt atomic.Value
}

func (p *promptRecorder) Name() string { return "fake" }
func (p *promptRecorder) Generate(_ context.Context, req engine.Request) (engine.Result, error) {
	p.calls.Add(1)
	p.prompt.Store(req.Prompt)
	return engine.Result{Text: p.out}, nil
}

// A draft that drops every limitation the writer was given is sent back with
// the rule named, and the rewrite that states one is accepted.
func TestADraftWithoutItsLimitationsIsRewritten(t *testing.T) {
	limitation := "The system was tested only on born-digital English papers."
	records := []claims.Record{
		{Claim: "The grounded result stands on its own.", SourceQuote: "The grounded result stands on its own", Type: "result", Strength: "demonstrated"},
		{Claim: limitation, SourceQuote: "tested only on born-digital English papers", Type: "limitation", Strength: "demonstrated"},
	}
	kept := validArticle(" " + limitation + " Scanned and non-English papers were tested only in part.")
	eng := &promptRecorder{out: kept}
	r := NewRunner(config.Config{WriteRetries: 1}, []engine.Engine{eng}, make(chan Event, 64))

	out, err := r.validateWithRetry(context.Background(), "base prompt", validArticle("."), records, rules.DefaultStyle())
	if err != nil {
		t.Fatalf("validateWithRetry = %v; the rewrite states the limitation", err)
	}
	if eng.calls.Load() != 1 || out != kept {
		t.Fatalf("made %d rewrite call(s); want one, returning the rewrite", eng.calls.Load())
	}
	if p, _ := eng.prompt.Load().(string); !strings.Contains(p, "states none of the 1 limitation") {
		t.Errorf("the rewrite prompt does not name the dropped limitation rule:\n%.400s", p)
	}
}

// The limitations the writer is given reach its prompt even when they come
// last in a long ledger, which the first-45 cut used to drop.
func TestTheWriterIsGivenLimitationsFromTheEndOfALongLedger(t *testing.T) {
	var records []claims.Record
	for i := 0; i < 80; i++ {
		records = append(records, claims.Record{Claim: "result " + strings.Repeat("x", i%7), SourceQuote: "quote", Type: "result", Strength: "demonstrated"})
	}
	records = append(records, claims.Record{Claim: "Only one hardware platform was measured.", SourceQuote: "one hardware platform", Type: "limitation", Strength: "demonstrated"})
	if !strings.Contains(promptLedger(records), "Only one hardware platform was measured.") {
		t.Error("the limitation at the end of the ledger did not reach the writer")
	}
	if got := writerLedger(records); len(got) != maxPromptClaims || got[len(got)-1].Type != "limitation" {
		t.Errorf("writerLedger = %d records ending in %q; want %d ending in the limitation", len(got), got[len(got)-1].Type, maxPromptClaims)
	}
}
