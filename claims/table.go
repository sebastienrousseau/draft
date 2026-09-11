// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"fmt"
	"regexp"
	"strings"
)

// tableSeparator matches a GitHub-flavoured markdown table's separator cell:
// dashes, optionally colon-anchored for alignment.
var tableSeparator = regexp.MustCompile(`^:?-+:?$`)

// TableClaims mines grounded claims from GitHub-flavoured markdown tables in
// source — the shape a layout reader (Docling) emits for a table, where a
// plain-text extractor would flatten the numbers into an unquotable jumble.
//
// Each numeric cell becomes one claim naming its row and column headers, with
// the verbatim data row it sits in as the quote. The value is read straight
// from the parsed cell, so the (row, column, value) association is grounded by
// construction; and because every candidate is then put through the same
// Verify as a model-extracted claim, a table claim is held to exactly the same
// gate — its quote must occur in the source and every number in the claim must
// occur in that quote. A cell with no number is not a claim and is skipped.
//
// This is additive: it never drops a model claim, only contributes facts a
// table states that the prose around it may not.
func TableClaims(source string) []Record {
	lines := strings.Split(source, "\n")
	var out []Record
	for i := 0; i < len(lines); {
		if !isTableRow(lines[i]) {
			i++
			continue
		}
		j := i
		for j < len(lines) && isTableRow(lines[j]) {
			j++
		}
		out = append(out, tableBlockClaims(lines[i:j], source)...)
		i = j
	}
	return out
}

// tableBlockClaims turns one contiguous run of table rows into claims. A run
// that is not a well-formed table (header, separator, then data) yields none.
func tableBlockClaims(rows []string, source string) []Record {
	if len(rows) < 3 || !isSeparatorRow(rows[1]) {
		return nil
	}
	header := splitRow(rows[0])
	var out []Record
	for _, row := range rows[2:] {
		cells := splitRow(row)
		if len(cells) < 2 {
			continue
		}
		rowHeader := cells[0]
		if rowHeader == "" {
			continue
		}
		quote := strings.TrimSpace(row)
		for j := 1; j < len(cells) && j < len(header); j++ {
			value, colHeader := cells[j], header[j]
			if colHeader == "" || len(Numbers(value)) == 0 {
				continue
			}
			rec := Record{
				Claim:       fmt.Sprintf("For %s, %s is %s.", rowHeader, colHeader, value),
				SourceQuote: quote,
				Type:        "metric",
				Strength:    "demonstrated",
			}
			if ok, _ := Verify(rec, source); ok {
				out = append(out, rec)
			}
		}
	}
	return out
}

// isTableRow reports whether a line is a markdown table row: trimmed, it opens
// with a pipe. Docling emits leading and trailing pipes on every row.
func isTableRow(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "|")
}

// isSeparatorRow reports whether a line is a table's header/body separator —
// every cell is dashes (with optional alignment colons).
func isSeparatorRow(line string) bool {
	cells := splitRow(line)
	for _, c := range cells {
		if !tableSeparator.MatchString(c) {
			return false
		}
	}
	return true
}

// splitRow splits a markdown table row into its trimmed cells, dropping the
// leading and trailing pipe delimiters.
func splitRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
