// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"fmt"
	"strings"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/provenance"
)

// LimitationRetention checks that the draft carries the limitations its
// writer was given: given is the ledger the writing prompt held (see
// claims.SelectForPrompt), not the whole extraction. A draft that keeps none
// of them is an error; each one dropped is a warning.
//
// "Carries" means what --verify reports: an article sentence attributed to
// the claim by provenance.Attribute. The floor is one, not all, because the
// measured rate is far below all: thirteen published articles carried 103 of
// the 310 limitations their ledgers held, and a writer given a long ledger
// cannot fit every caveat into one article. A draft with none, though, is
// presenting a source's results without the limits the source states.
func LimitationRetention(article string, given []claims.Record) (errs, warnings []string) {
	dropped, total := droppedLimitations(article, given)
	if total == 0 {
		return nil, nil
	}
	for _, rec := range dropped {
		warnings = append(warnings, "limitation not carried into the article: \""+snippet(rec.Claim, 90)+"\"")
	}
	if len(dropped) == total {
		errs = append(errs, fmt.Sprintf("article states none of the %d limitation(s) in its claims; state at least one, in the source's terms", total))
	}
	return errs, warnings
}

// LimitationCounts is how many limitations the writer was given and how many
// the draft carries, for the provenance record.
func LimitationCounts(article string, given []claims.Record) (total, kept int) {
	dropped, total := droppedLimitations(article, given)
	return total, total - len(dropped)
}

func droppedLimitations(article string, given []claims.Record) (dropped []claims.Record, total int) {
	carried := map[string]bool{}
	for _, s := range provenance.Attribute(article, given).Sentences {
		for _, id := range s.Claims {
			carried[id] = true
		}
	}
	for _, rec := range given {
		if !strings.EqualFold(strings.TrimSpace(rec.Type), "limitation") {
			continue
		}
		total++
		if !carried[provenance.ClaimID(rec)] {
			dropped = append(dropped, rec)
		}
	}
	return dropped, total
}
