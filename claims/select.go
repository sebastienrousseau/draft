// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"fmt"
	"slices"
	"strings"
)

// minLimitationSlots is the smallest reserve of writer slots for limitations.
const minLimitationSlots = 3

// SelectForPrompt chooses the records the writer is given when the ledger
// does not fit the prompt budget: at most maxClaims records whose rendered
// blocks, with the header, fit in maxChars. A ledger that fits is returned
// whole and in order.
//
// It used to be the first records in source order, which on a long paper is
// the abstract and introduction: across thirteen published articles, the
// writer of a 514-claim ledger was given claims from its opening sections
// only, and limitations, which papers state near the end, rarely reached it.
// So the selection is spread evenly across the ledger, and a fifth of the
// slots (at least minLimitationSlots) is reserved for limitation claims, so
// the writer sees what the source says it does not show. Slots the other
// records cannot fill go to more limitations. The character budget drops
// other records before limitations.
//
// The result is in source order and deterministic, so every writing prompt
// of a run, and the checks on its output, see the same selection.
func SelectForPrompt(records []Record, maxClaims, maxChars int) []Record {
	if maxClaims <= 0 {
		return nil
	}
	var lims, rest []int
	for i, r := range records {
		if isLimitation(r) {
			lims = append(lims, i)
		} else {
			rest = append(rest, i)
		}
	}
	limSlots := min(len(lims), maxClaims, max(minLimitationSlots, maxClaims/5))
	chosenLims := spread(lims, limSlots)
	chosenRest := spread(rest, maxClaims-len(chosenLims))
	if spare := maxClaims - len(chosenLims) - len(chosenRest); spare > 0 {
		chosenLims = append(chosenLims, spread(without(lims, chosenLims), spare)...)
	}
	return budgeted(records, chosenLims, chosenRest, maxChars)
}

func isLimitation(r Record) bool {
	return strings.EqualFold(strings.TrimSpace(r.Type), "limitation")
}

// spread picks k of idx at evenly spaced, centred positions, in order.
func spread(idx []int, k int) []int {
	if k >= len(idx) {
		return slices.Clone(idx)
	}
	out := make([]int, 0, k)
	for j := 0; j < k; j++ {
		out = append(out, idx[(2*j+1)*len(idx)/(2*k)])
	}
	return out
}

// without is idx minus the members of drop.
func without(idx, drop []int) []int {
	var out []int
	for _, i := range idx {
		if !slices.Contains(drop, i) {
			out = append(out, i)
		}
	}
	return out
}

// budgeted keeps the chosen records that fit in maxChars, limitations first,
// skipping any that would overflow, and returns them in source order.
func budgeted(records []Record, lims, rest []int, maxChars int) []Record {
	used := promptHeaderLen(len(records))
	var kept []int
	for _, i := range append(slices.Clone(lims), rest...) {
		if n := blockLen(records[i]); used+n <= maxChars {
			used += n
			kept = append(kept, i)
		}
	}
	slices.Sort(kept)
	out := make([]Record, len(kept))
	for k, i := range kept {
		out[k] = records[i]
	}
	return out
}

// promptHeaderLen is the length of the compact ledger's header for a ledger
// of total records, sized for its widest count line.
func promptHeaderLen(total int) int {
	return len(promptLedgerHeader) + len(fmt.Sprintf("Included records: %d of %d\n\n", total, total))
}

// blockLen is the rendered length of rec in the compact ledger.
func blockLen(rec Record) int {
	var b strings.Builder
	writeRecord(&b, rec)
	return b.Len()
}
