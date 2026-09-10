// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package claims extracts, verifies, de-duplicates, and renders the verified
// claim ledger that grounds every draft. A claim survives only if its
// SOURCE_QUOTE is an exact substring of the section it was drawn from and every
// number in the claim also appears in that quote. This is the single most
// important defence against a model inventing plausible facts.
package claims

import (
	"regexp"
	"strings"
)

// Record is one verified fact plus the verbatim span that supports it.
type Record struct {
	Claim       string
	SourceQuote string
	Type        string
	Strength    string
}

var (
	blockSep     = regexp.MustCompile(`(?m)^---\s*$`)
	danglingTail = regexp.MustCompile(`(?i)(?:,|;|\b(?:and|or|but|the|a|an|of|to|in|for|with|that|which|is|are|was|were))\s*$`)
	numberPat    = regexp.MustCompile(`\d+(?:[.,]\d+)*`)
	spacePat     = regexp.MustCompile(`\s+`)
	smartQuotes  = strings.NewReplacer("“", `"`, "”", `"`, "‘", "'", "’", "'")
)

// Parse reads a model's extraction output for a single source section and
// returns the records whose quotes verify, plus the count that were dropped.
func Parse(text, source string) (records []Record, dropped int) {
	text = strings.TrimSpace(text)
	if text == "" || text == "NONE" {
		return nil, 0
	}
	for _, block := range blockSep.Split(text, -1) {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		rec := Record{
			Claim:       fieldValue(block, "CLAIM"),
			SourceQuote: strings.Trim(fieldValue(block, "SOURCE_QUOTE"), `"`),
			Type:        fieldValue(block, "TYPE"),
			Strength:    fieldValue(block, "STRENGTH"),
		}
		// The common case is a record that already verifies, so it pays for one
		// Verify and nothing else. Repair — which allocates — runs only when the
		// record would otherwise be dropped, and its result is re-verified so a
		// rescued quote is held to exactly the same gate.
		if ok, _ := Verify(rec, source); ok {
			records = append(records, rec)
			continue
		}
		if repaired := repair(rec, source); repaired.SourceQuote != rec.SourceQuote {
			if ok, _ := Verify(repaired, source); ok {
				records = append(records, repaired)
				continue
			}
		}
		dropped++
	}
	return records, dropped
}

// ParseLedger reads back a ledger previously written by RenderLedger,
// re-verifying every record against source exactly as Parse does.
//
// A resumed ledger is not trusted because we wrote it; it is trusted because it
// still passes the same gate. That is what lets a run reuse extraction work
// without weakening grounding: if a source has changed underneath it, the
// records that no longer occur in it are dropped here.
//
// RenderLedger writes a human-readable header before the first record. Parse
// would read that as a block with no CLAIM and count it as a drop, so it is
// skipped: the returned count reflects records that genuinely no longer verify.
func ParseLedger(ledger, source string) (records []Record, dropped int) {
	i := strings.Index(ledger, "CLAIM:")
	if i < 0 {
		// No records at all — an empty ledger renders as a header and "NONE".
		// Parse would read that header as one malformed block and report a
		// phantom drop.
		return nil, 0
	}
	return Parse(ledger[i:], source)
}

// Dedupe removes records whose normalised claim text has already been seen.
func Dedupe(records []Record) []Record {
	seen := map[string]bool{}
	out := make([]Record, 0, len(records))
	for _, rec := range records {
		key := strings.ToLower(strings.TrimSpace(spacePat.ReplaceAllString(rec.Claim, " ")))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, rec)
	}
	return out
}
