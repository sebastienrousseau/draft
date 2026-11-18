// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/rules"
)

// adoptSourceTerms returns style without the banned terms the verified quotes
// use, and records them so finalize leaves them alone. Both halves matter: a
// rewrite that kept a term the validator still banned would send a faithful
// draft back for a retry it cannot pass.
func (r *Runner) adoptSourceTerms(style rules.Style, records []claims.Record) rules.Style {
	quotes := make([]string, 0, len(records))
	for _, rec := range records {
		quotes = append(quotes, rec.SourceQuote)
	}
	style, r.sourceTerms = style.ExemptSourceTerms(quotes)
	return style
}

// reviewViolationsBoth judges the draft before and after a review against the
// same style, with the sources' own terms exempted, so a review is measured by
// what its edit introduced.
func (r *Runner) reviewViolationsBoth(before, after string, records []claims.Record) (b, a []string) {
	style := r.adoptSourceTerms(r.cfg.Style, records)
	return reviewViolations(before, records, style, r.reviewOptions()),
		reviewViolations(after, records, style, r.reviewOptions())
}
