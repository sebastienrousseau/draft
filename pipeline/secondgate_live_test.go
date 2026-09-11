// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
)

// TestSecondGateLiveOllama proves the real entailment path works against a
// local model. It is opt-in — set DRAFT_LIVE_OLLAMA=1 with an Ollama server
// running the default model — so it never runs in CI or by accident, only when
// a developer deliberately exercises the live path.
func TestSecondGateLiveOllama(t *testing.T) {
	if os.Getenv("DRAFT_LIVE_OLLAMA") == "" {
		t.Skip("set DRAFT_LIVE_OLLAMA=1 with a running Ollama server to run the live second-gate check")
	}
	cfg := config.Config{
		OllamaHost:    config.OllamaHost,
		OllamaModel:   config.DefaultOllamaModel,
		ExtractModel:  config.DefaultExtractModel,
		EditModel:     config.DefaultEditModel,
		ContextLength: config.DefaultContextLen,
		PredictLength: 64,
		CallTimeout:   3 * time.Minute,
	}
	r := NewRunner(cfg, []engine.Engine{engine.NewOllama(cfg)}, nil)
	e := engineEntailer{r}
	ctx := context.Background()

	// A quote that plainly states the claim is supported.
	ok, err := e.Supports(ctx, "The method improves accuracy.",
		"the method improves accuracy across all benchmarks")
	if err != nil {
		t.Fatalf("live entailment (supported) errored: %v", err)
	}
	if !ok {
		t.Error("live model marked a plainly-supported claim UNSUPPORTED")
	}

	// A quote that states the opposite: verbatim overlap, contradicted meaning.
	ok, err = e.Supports(ctx, "The method improves accuracy.",
		"the method did not improve accuracy in any setting")
	if err != nil {
		t.Fatalf("live entailment (contradicted) errored: %v", err)
	}
	if ok {
		t.Error("live model marked a contradicted claim SUPPORTED")
	}
}
