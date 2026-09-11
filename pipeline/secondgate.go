// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/prompt"
)

// engineEntailer is the second gate's verifier: it asks the run's engine
// whether a claim's quote supports it, using the edit chain (a small, cheap
// judgement, kept off the writing model's critical path). It implements
// claims.Entailer.
type engineEntailer struct{ r *Runner }

// Supports asks the model for a one-word verdict and reads it. A model or
// transport error is returned so SecondPass can keep the claim (fail-open); an
// unreadable verdict is treated as support, so the gate only ever drops a
// claim the model clearly marks unsupported.
func (e engineEntailer) Supports(ctx context.Context, claim, quote string) (bool, error) {
	res, err := e.r.generate(ctx, engine.Request{
		Kind:        engine.KindEdit,
		Prompt:      prompt.Entailment(claim, quote),
		Temperature: editTemperature,
		// The verdict is a single word. Cap the output so a local model cannot
		// spend minutes generating past a one-word answer (session providers
		// ignore NumPredict and answer briefly anyway).
		NumPredict: 16,
	})
	if err != nil {
		return false, err
	}
	return verdictSupported(res.Text), nil
}

// verdictSupported reads the second gate's one-word answer. It drops a claim
// only on a clear "unsupported"; anything else — "supported", or a verdict it
// cannot read — keeps it, so an evasive or malformed answer never silently
// thins the ledger.
func verdictSupported(text string) bool {
	t := strings.ToLower(text)
	return !strings.Contains(t, "unsupported")
}

// secondGate runs the opt-in semantic second pass over records when enabled,
// returning the surviving records. It is a no-op unless cfg.SecondGate is set.
func (r *Runner) secondGate(ctx context.Context, records []claims.Record) []claims.Record {
	if !r.cfg.SecondGate || len(records) == 0 {
		return records
	}
	kept, dropped := claims.SecondPass(ctx, records, engineEntailer{r})
	if dropped > 0 {
		r.log(fmt.Sprintf("second gate dropped %d claim(s) a model found unsupported by their quote", dropped))
	}
	return kept
}
