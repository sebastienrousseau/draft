// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"fmt"
	"strings"
)

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
	rec = singleLine(rec)
	b.WriteString("CLAIM: " + rec.Claim + "\n")
	b.WriteString("SOURCE_QUOTE: \"" + rec.SourceQuote + "\"\n")
	b.WriteString("TYPE: " + rec.Type + "\n")
	b.WriteString("STRENGTH: " + rec.Strength + "\n")
	b.WriteString("---\n")
}

// singleLine puts every field of rec on one line.
//
// The ledger is line-structured: a line opening with a field name starts that
// field, and a "---" line ends the record. So a field that carried its own line
// breaks could close its record and open a forged one, whose quote nobody
// checked, inside the block the writer is told is verified. With every field
// on one line, a rendered ledger has exactly one record per record written.
//
// A quote that spans lines is stored canonicalised rather than merely joined:
// canonical rejoins a word the extractor hyphenated across a line break, which
// a plain join would leave as "evalu- ation", a quote no longer in the source.
func singleLine(rec Record) Record {
	rec.Claim = joinFields(rec.Claim)
	rec.Type = joinFields(rec.Type)
	rec.Strength = joinFields(rec.Strength)
	if strings.ContainsFunc(rec.SourceQuote, isLineBreak) {
		rec.SourceQuote = canonical(rec.SourceQuote)
	}
	return rec
}

func joinFields(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// isLineBreak reports the characters any reader of the ledger might take as
// the end of a line. Each is also a space to strings.Fields, so canonical
// removes all of them.
func isLineBreak(r rune) bool {
	switch r {
	case '\n', '\r', '\v', '\f', '\u0085', ' ', ' ':
		return true
	}
	return false
}

// spellsField reports whether a claim contains a ledger field name. A claim is
// the model's own sentence, so one that writes "SOURCE_QUOTE:" is formatting a
// record, not stating a fact, and the writer would read it as one even on a
// single line. Quotes are not refused this way: a quote is the source's words,
// held to the source by Verify.
func spellsField(claim string) bool {
	for _, name := range []string{"CLAIM:", "SOURCE_QUOTE:", "TYPE:", "STRENGTH:"} {
		if strings.Contains(claim, name) {
			return true
		}
	}
	return false
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
