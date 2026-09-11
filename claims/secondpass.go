// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import "context"

// Entailer judges whether a record's claim is semantically supported by its
// own verified source quote — the meaning check the verbatim gate documents it
// does not make. An implementation typically asks a local model.
type Entailer interface {
	// Supports reports whether quote supports claim. An error means the check
	// could not be made (a model was unreachable, say), which SecondPass treats
	// as "keep", never "drop".
	Supports(ctx context.Context, claim, quote string) (bool, error)
}

// SecondPass runs each record's (claim, quote) through the entailer and returns
// the records it supports plus the count it dropped. It is the opt-in second
// gate: strictly additive on top of the verbatim gate, which has already
// passed every record here, and never the default.
//
// It is fail-open. A record is dropped only on a confident, error-free
// "unsupported"; an entailer error keeps the record, so a flaky or offline
// verifier can never silently thin a run's grounded material — the worst it can
// do is decline to tighten it. A cancelled context stops the pass and keeps the
// remaining records untouched.
func SecondPass(ctx context.Context, records []Record, e Entailer) (kept []Record, dropped int) {
	for i, r := range records {
		if ctx.Err() != nil {
			// Do not silently drop the rest on cancellation: keep them.
			kept = append(kept, records[i:]...)
			return kept, dropped
		}
		ok, err := e.Supports(ctx, r.Claim, r.SourceQuote)
		if err != nil || ok {
			kept = append(kept, r)
			continue
		}
		dropped++
	}
	return kept, dropped
}
