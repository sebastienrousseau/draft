// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"errors"
	"fmt"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/pdf"
)

// ErrNoVerifiedClaims is returned when no claim survives verification. The
// writer may state only verified claims, so with none there is nothing to
// write. The faithfulness checks see only numbers and metric terms, so an
// article written anyway would pass them on prose grounded in nothing.
var ErrNoVerifiedClaims = errors.New("no claim survived verification")

// groundClaims is phase 2: extract claims section by section, unless a ledger
// from an earlier attempt is on disk and still verifies, and render the
// compact ledger the writer is given. It fails rather than return an empty
// ledger.
func (r *Runner) groundClaims(ctx context.Context, job Job, sections []pdf.Section, outputDir string) ([]claims.Record, string, error) {
	r.phase(PhaseClaims, "running")
	records, dropped, err := r.resumeOrExtract(ctx, job, sections, outputDir, r.chainFor(engine.KindExtract))
	if err != nil {
		r.phase(PhaseClaims, "failed")
		return nil, "", err
	}
	r.log(fmt.Sprintf("verified %d claim(s), dropped %d", len(records), dropped))
	if len(records) == 0 {
		r.phase(PhaseClaims, "failed")
		return nil, "", fmt.Errorf("%w (%d dropped): nothing grounded to write from; "+
			"try --reader docling for tables or scanned pages, or another --extract-engine", ErrNoVerifiedClaims, dropped)
	}
	r.phase(PhaseClaims, "done")
	return records, claims.RenderPromptLedger(records, maxPromptClaims, maxPromptClaimChars), nil
}
