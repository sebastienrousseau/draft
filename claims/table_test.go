// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"strings"
	"testing"
)

// doclingTable is the verbatim Markdown Docling emits for a small results
// table (captured from `docling --to md`), padding and all. Using the real
// output keeps the parser honest about the format it actually has to read.
const doclingTable = `# Results

Table 1 reports throughput.

| Model    |   Throughput (pages/s) |   Accuracy |
|----------|------------------------|------------|
| draft-A  |                     99 |       0.82 |
| baseline |                     33 |       0.71 |
`

func TestTableClaimsFromRealDoclingOutput(t *testing.T) {
	recs := TableClaims(doclingTable)
	// Two data rows × two numeric columns = four cell claims; the text "Model"
	// column is the row header, not a value.
	if len(recs) != 4 {
		t.Fatalf("got %d claims, want 4:\n%+v", len(recs), recs)
	}
	want := map[string]bool{
		"For draft-A, Throughput (pages/s) is 99.":  false,
		"For draft-A, Accuracy is 0.82.":            false,
		"For baseline, Throughput (pages/s) is 33.": false,
		"For baseline, Accuracy is 0.71.":           false,
	}
	for _, r := range recs {
		if _, ok := want[r.Claim]; !ok {
			t.Errorf("unexpected claim %q", r.Claim)
			continue
		}
		want[r.Claim] = true
		// Every emitted claim must pass the ordinary gate, unchanged.
		if ok, why := Verify(r, doclingTable); !ok {
			t.Errorf("emitted claim fails Verify (%s): %q", why, r.Claim)
		}
		if r.Type != "metric" || r.Strength != "demonstrated" {
			t.Errorf("wrong type/strength on %q: %s/%s", r.Claim, r.Type, r.Strength)
		}
	}
	for claim, seen := range want {
		if !seen {
			t.Errorf("missing expected claim %q", claim)
		}
	}
}

func TestTableClaimsSkipsNonNumericAndMalformed(t *testing.T) {
	cases := map[string]string{
		"no table":            "Just prose with | a stray pipe but no table.",
		"header only":         "| A | B |\n|---|---|",
		"text-only cells":     "| Name | Role |\n|------|------|\n| Ann | lead |\n| Bo | eng |",
		"missing separator":   "| A | B |\n| 1 | 2 |",
		"no separator 3 rows": "| A | B |\n| x | y |\n| 1 | 2 |",
		"empty":               "",
		"pipes but no header": "no pipes here at all",
	}
	for name, src := range cases {
		if got := TableClaims(src); len(got) != 0 {
			t.Errorf("%s: got %d claims, want 0:\n%+v", name, len(got), got)
		}
	}
}

func TestTableClaimsMixedCells(t *testing.T) {
	// A table where only some columns are numeric: only the numeric cells
	// become claims, and a blank cell is skipped.
	src := "| Method | Score | Notes |\n" +
		"|--------|-------|-------|\n" +
		"| fast   | 0.91  | good  |\n" +
		"| slow   |       | n/a   |\n"
	recs := TableClaims(src)
	if len(recs) != 1 {
		t.Fatalf("got %d claims, want 1 (only fast/Score):\n%+v", len(recs), recs)
	}
	if recs[0].Claim != "For fast, Score is 0.91." {
		t.Errorf("claim = %q", recs[0].Claim)
	}
}

// A claim whose value carries a number absent from its own data row cannot
// arise from a real cell, but the Verify pass is the backstop that guarantees
// it: every emitted claim's numbers are a subset of its quoted row.
func TestTableClaimsAllPassVerify(t *testing.T) {
	src := "| Model | Top-5 acc | Params (B) |\n" +
		"|-------|-----------|------------|\n" +
		"| M1    | 88        | 7          |\n"
	for _, r := range TableClaims(src) {
		if ok, why := Verify(r, src); !ok {
			t.Errorf("claim %q does not verify: %s", r.Claim, why)
		}
		// The column header "Top-5 acc" carries a 5; the claim then contains a
		// 5, and it must be present in the quoted row or Verify would drop it.
		if strings.Contains(r.Claim, "Top-5") {
			if nums := Numbers(r.SourceQuote); !nums["5"] {
				// Not necessarily present — if it is not, Verify must have
				// dropped the claim, so reaching here means it was kept and
				// the quote must contain the 5.
				t.Errorf("kept a Top-5 claim whose row lacks a 5: quote=%q", r.SourceQuote)
			}
		}
	}
}

// Degenerate data rows — an empty row header, or a row with too few cells —
// are skipped rather than emitted.
func TestTableClaimsSkipsDegenerateRows(t *testing.T) {
	src := "| Model | Score |\n" +
		"|-------|-------|\n" +
		"|       | 5     |\n" + // empty row header -> skip
		"| solo\n" + // ragged row that splits to one cell -> skip
		"| M1    | 7     |\n" // the only real claim
	recs := TableClaims(src)
	if len(recs) != 1 || recs[0].Claim != "For M1, Score is 7." {
		t.Fatalf("got %d claims, want 1 (For M1, Score is 7.):\n%+v", len(recs), recs)
	}
}
