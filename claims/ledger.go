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
