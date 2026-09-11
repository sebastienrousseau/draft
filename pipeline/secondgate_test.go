// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"testing"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
)

func TestVerdictSupported(t *testing.T) {
	for text, want := range map[string]bool{
		"SUPPORTED":                 true,
		"supported":                 true,
		"UNSUPPORTED":               false,
		"  unsupported\n":           false,
		"The claim is UNSUPPORTED.": false,
		"":                          true, // unreadable -> keep
		"maybe":                     true, // ambiguous -> keep
		"SUPPORTED, clearly":        true,
	} {
		if got := verdictSupported(text); got != want {
			t.Errorf("verdictSupported(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestSecondGateDisabledIsNoop(t *testing.T) {
	in := []claims.Record{{Claim: "a", SourceQuote: "q"}}
	r := NewRunner(config.Config{}, []engine.Engine{&countingEngine{name: "x", out: "UNSUPPORTED"}}, nil)
	// SecondGate defaults false: the engine must not even be consulted.
	out := r.secondGate(context.Background(), in)
	if len(out) != 1 {
		t.Errorf("disabled gate changed the ledger: %d records", len(out))
	}
}

func TestSecondGateDropsUnsupportedViaEngine(t *testing.T) {
	in := []claims.Record{{Claim: "a", SourceQuote: "q1"}, {Claim: "b", SourceQuote: "q2"}}
	cfg := config.Config{SecondGate: true}
	// The engine answers UNSUPPORTED for every entailment prompt.
	r := NewRunner(cfg, []engine.Engine{&countingEngine{name: "x", out: "UNSUPPORTED"}}, nil)
	out := r.secondGate(context.Background(), in)
	if len(out) != 0 {
		t.Errorf("gate kept %d records, want 0 (engine said UNSUPPORTED)", len(out))
	}
}

func TestSecondGateKeepsSupportedViaEngine(t *testing.T) {
	in := []claims.Record{{Claim: "a", SourceQuote: "q1"}}
	cfg := config.Config{SecondGate: true}
	r := NewRunner(cfg, []engine.Engine{&countingEngine{name: "x", out: "SUPPORTED"}}, nil)
	if out := r.secondGate(context.Background(), in); len(out) != 1 {
		t.Errorf("gate dropped a supported claim: %d records", len(out))
	}
}

// When the engine errors, Supports surfaces the error so SecondPass keeps the
// claim (fail-open) rather than dropping it.
func TestEngineEntailerSurfacesError(t *testing.T) {
	boom := funcEngine{name: "x", gen: func(context.Context, engine.Request) (engine.Result, error) {
		return engine.Result{}, context.DeadlineExceeded
	}}
	r := NewRunner(config.Config{SecondGate: true}, []engine.Engine{boom}, nil)
	if _, err := (engineEntailer{r}).Supports(context.Background(), "c", "q"); err == nil {
		t.Error("Supports should surface the engine error")
	}
	// Fail-open at the gate: the claim survives despite the engine error.
	kept := r.secondGate(context.Background(), []claims.Record{{Claim: "c", SourceQuote: "q"}})
	if len(kept) != 1 {
		t.Errorf("gate dropped a claim on engine error; want fail-open keep")
	}
}
