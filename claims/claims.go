// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package claims extracts, verifies, de-duplicates, and renders the verified
// claim ledger that grounds every draft. A claim survives only if its
// SOURCE_QUOTE is an exact substring of the section it was drawn from and every
// number in the claim also appears in that quote. This is the single most
// important defence against a model inventing plausible facts.
package claims

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/sebastienrousseau/draft/rules"
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
		if ok, _ := Verify(rec, source); !ok {
			dropped++
			continue
		}
		records = append(records, rec)
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

// Verify reports whether a record is trustworthy and, when not, why.
func Verify(rec Record, source string) (bool, string) {
	quote := strings.TrimSpace(rec.SourceQuote)
	if rec.Claim == "" || quote == "" {
		return false, "missing claim or quote"
	}
	if len([]rune(quote)) < rules.MinQuoteChars {
		return false, "quote too short"
	}
	// A quote must be well-formed text before it can be compared.
	//
	// Comparison normalises with strings.ToLower, which maps every invalid
	// UTF-8 byte to U+FFFD — so two DIFFERENT invalid byte sequences normalise
	// to the same string, and a fabricated quote could match a source it does
	// not occur in. Found by FuzzParse. Requiring the quote to be valid UTF-8
	// and free of U+FFFD closes that: normalisation can only ever introduce
	// U+FFFD, never other characters, so a quote containing none of it cannot
	// be matched against mangled bytes.
	if !utf8.ValidString(quote) {
		return false, "quote is not valid UTF-8"
	}
	if strings.ContainsRune(quote, utf8.RuneError) {
		return false, "quote contains a replacement character"
	}
	if !quoteInSource(quote, source) {
		return false, "quote not found in source"
	}
	if danglingTail.MatchString(quote) {
		return false, "quote is a truncated fragment"
	}
	recType := strings.ToLower(strings.TrimSpace(rec.Type))
	if recType != "" && !rules.ClaimTypes[recType] {
		return false, "invalid TYPE '" + recType + "'"
	}
	strength := strings.ToLower(strings.TrimSpace(rec.Strength))
	if strength != "" && !rules.ClaimStrengths[strength] {
		return false, "invalid STRENGTH '" + strength + "'"
	}

	quoteNums := Numbers(quote)
	var missing []string
	for n := range Numbers(rec.Claim) {
		if !quoteNums[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return false, "claim numbers absent from quote: " + strings.Join(missing, ", ")
	}
	return true, ""
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

// Numbers returns the distinct numeric tokens in s, with thousands separators
// stripped so "1,000" and "1000" compare equal.
func Numbers(s string) map[string]bool {
	out := map[string]bool{}
	for _, n := range numberPat.FindAllString(s, -1) {
		out[strings.ReplaceAll(n, ",", "")] = true
	}
	return out
}

// RenderLedger produces the full, human-readable verified claim ledger.
func RenderLedger(records []Record, dropped int) string {
	if len(records) == 0 {
		return "# Verified Claim Ledger\n\nNONE\n"
	}
	var b strings.Builder
	b.WriteString("# Verified Claim Ledger\n\n")
	fmt.Fprintf(&b, "Verified records: %d\n", len(records))
	fmt.Fprintf(&b, "Dropped records with unverifiable SOURCE_QUOTE: %d\n\n", dropped)
	for _, rec := range records {
		writeRecord(&b, rec)
	}
	return b.String()
}

// RenderPromptLedger produces the compact ledger fed to the writing model,
// capped by record count and character budget so a small model is not swamped.
func RenderPromptLedger(records []Record, maxClaims, maxChars int) string {
	var b strings.Builder
	b.WriteString("# Compact Verified Claims For Writing\n\n")
	countPos := b.Len()
	b.WriteString("Included records: 0 of 0\n\n")
	included := 0
	for _, rec := range records {
		var block strings.Builder
		writeRecord(&block, rec)
		if included >= maxClaims || b.Len()+block.Len() > maxChars {
			break
		}
		b.WriteString(block.String())
		included++
	}
	out := b.String()
	countLine := fmt.Sprintf("Included records: %d of %d", included, len(records))
	nl := strings.Index(out[countPos:], "\n") + countPos
	return out[:countPos] + countLine + out[nl:]
}

func writeRecord(b *strings.Builder, rec Record) {
	b.WriteString("CLAIM: " + rec.Claim + "\n")
	b.WriteString("SOURCE_QUOTE: \"" + rec.SourceQuote + "\"\n")
	b.WriteString("TYPE: " + rec.Type + "\n")
	b.WriteString("STRENGTH: " + rec.Strength + "\n")
	b.WriteString("---\n")
}

func fieldValue(block, field string) string {
	prefix := field + ":"
	var out []string
	capturing := false
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if isFieldHeader(trimmed) {
			if capturing {
				break
			}
			if strings.HasPrefix(trimmed, prefix) {
				capturing = true
				out = append(out, strings.TrimSpace(strings.TrimPrefix(trimmed, prefix)))
			}
			continue
		}
		if capturing {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func isFieldHeader(trimmed string) bool {
	return strings.HasPrefix(trimmed, "CLAIM:") ||
		strings.HasPrefix(trimmed, "SOURCE_QUOTE:") ||
		strings.HasPrefix(trimmed, "TYPE:") ||
		strings.HasPrefix(trimmed, "STRENGTH:")
}

func quoteInSource(quote, source string) bool {
	return strings.Contains(normalise(source), normalise(quote))
}

func normalise(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(smartQuotes.Replace(s)), " "))
}
