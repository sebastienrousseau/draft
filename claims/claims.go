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
	// numberPat uses \p{Nd} rather than \d, which is ASCII-only in RE2: a
	// claim written in another script's digits would otherwise carry no
	// numbers at all and skip the check that its numbers are in its quote.
	numberPat = regexp.MustCompile(`\p{Nd}+(?:[.,]\p{Nd}+)*`)
	// thousandsPat is a number whose commas only group digits — Western
	// (1,000,000) or Indian (10,00,000) — and so may be dropped.
	thousandsPat = regexp.MustCompile(`^(?:\d{1,3}(?:,\d{3})+|\d{1,2}(?:,\d{2})+,\d{3})(?:\.\d+)?$`)
	spacePat     = regexp.MustCompile(`\s+`)
)

// Parse reads a model's extraction output for a single source section and
// returns the records whose quotes verify, plus the count that were dropped.
func Parse(text, source string) (records []Record, dropped int) {
	text = strings.TrimSpace(text)
	if text == "" || text == "NONE" {
		return nil, 0
	}
	if strings.HasPrefix(text, "[") {
		return ParseJSON(text, source)
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
		if verified, ok := verifyOrRepair(rec, source); ok {
			records = append(records, verified)
			continue
		}
		dropped++
	}
	return records, dropped
}

// verifyOrRepair checks rec against source, attempting one quote-extension
// repair when the initial check fails and re-verifying the repaired record to
// the same standard.
//
// The record is put on one line per field first, so what is verified is
// exactly what the ledger will hold, and a claim that spells out a ledger field
// is refused outright: see singleLine.
func verifyOrRepair(rec Record, source string) (Record, bool) {
	rec = singleLine(rec)
	if spellsField(rec.Claim) {
		return Record{}, false
	}
	if ok, _ := Verify(rec, source); ok {
		return rec, true
	}
	if repaired := repair(rec, source); repaired.SourceQuote != rec.SourceQuote {
		if ok, _ := Verify(repaired, source); ok {
			return repaired, true
		}
	}
	return Record{}, false
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

// ParseLedgerSections is ParseLedger against sources kept apart: a record is
// kept when its quote verifies in any one section.
//
// Verifying against the sections joined together is weaker than a fresh run,
// which verifies each section's claims against that section alone.
// Normalisation folds the separator between two sections into a space, so a
// quote running from the end of one into the start of the next is found in
// the joined text while occurring in no source.
func ParseLedgerSections(ledger string, sections []string) (records []Record, dropped int) {
	i := strings.Index(ledger, "CLAIM:")
	if i < 0 {
		// An empty ledger renders as a header and "NONE"; see ParseLedger.
		return nil, 0
	}
	for _, block := range blockSep.Split(ledger[i:], -1) {
		if strings.TrimSpace(block) == "" {
			continue
		}
		if rec, ok := firstSupporting(block, sections); ok {
			records = append(records, rec)
			continue
		}
		dropped++
	}
	return records, dropped
}

// firstSupporting verifies one ledger block against each section in turn and
// returns the record from the first that supports it.
func firstSupporting(block string, sections []string) (Record, bool) {
	for _, s := range sections {
		if recs, _ := Parse(block, s); len(recs) == 1 {
			return recs[0], true
		}
	}
	return Record{}, false
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
