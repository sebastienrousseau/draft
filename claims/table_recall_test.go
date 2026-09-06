// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebastienrousseau/draft/claims"
)

// Table recall measures what the document reader is worth on tabular sources.
//
// The audit's open item A3 was that plain-text extraction flattens a table:
// pdftotext reads a table column by column, so a row label and its value land
// far apart and a claim that quotes them together can never be verified. A
// layout reader (Docling) emits the table as Markdown with each row intact, so
// the same claim's quote occurs verbatim and grounds.
//
// The two fixtures are the same table as each reader renders it. The claims
// quote a cell together with its row label — exactly the shape a table exists
// to support. The measurement is the recall gap: how many of those claims
// verify against each rendering. It is deterministic and needs no reader
// installed, so it runs in CI as a guard on the value the reader adds.
func TestTableRecallStructuredBeatsFlattened(t *testing.T) {
	structured := readFixture(t, "structured.md")
	flattened := readFixture(t, "flattened.txt")

	// Each claim pairs a row label with a cell value from the same row.
	cellClaims := []struct{ claim, quote string }{
		{"B&H returned -5.23 percent on AAPL", `| Market     | B&H      | -5.23`},
		{"MACD returned 6.20 percent on GOOGL", `| Rule-based | MACD     | -1.49    | 6.20`},
		{"AlphaMix returned 21.3 percent on AMZN", `| AlphaMix | 12.4     | 9.1       | 21.3     |`},
	}

	structuredVerified, flattenedVerified := 0, 0
	for _, c := range cellClaims {
		if ok, _ := claims.Verify(claims.Record{Claim: c.claim, SourceQuote: c.quote}, structured); ok {
			structuredVerified++
		}
		// The same quote cannot occur in the flattened text, because the reader
		// scattered the row's cells; so a claim of the same fact cannot ground.
		if ok, _ := claims.Verify(claims.Record{Claim: c.claim, SourceQuote: c.quote}, flattened); ok {
			flattenedVerified++
		}
	}

	if structuredVerified != len(cellClaims) {
		t.Errorf("structured reader recall = %d/%d; a layout reader should keep every row quotable",
			structuredVerified, len(cellClaims))
	}
	if flattenedVerified != 0 {
		t.Errorf("flattened reader recall = %d; the plain-text flow should scatter every table row", flattenedVerified)
	}

	// The prose outside the table grounds under both readers: the reader only
	// changes recall on tabular structure, not on running text.
	proseQuote := "The AlphaMix model led on every ticker."
	if !quoteVerifies(proseQuote, structured) || !quoteVerifies(proseQuote, flattened) {
		t.Error("prose should ground under either reader")
	}
}

func quoteVerifies(quote, source string) bool {
	ok, _ := claims.Verify(claims.Record{Claim: quote, SourceQuote: quote}, source)
	return ok
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "tables", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
