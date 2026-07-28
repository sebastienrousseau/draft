// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pdf

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
)

// gapLine matches a line carrying a wide run of interior whitespace — the
// signature of two columns spliced onto one line by a layout-preserving
// extractor.
var gapLine = regexp.MustCompile(`\S {4,}\S`)

// The fixture (testdata/two-column.pdf, built by make_fixture.py) mimics the
// paper shape that broke extraction in production: a contents list naming
// "References" on page 1, body prose in two columns, and the real
// bibliography on the last page.

func TestExtractReadsColumnsInReadingOrder(t *testing.T) {
	text, err := Extract(context.Background(), "testdata/two-column.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(gapLine.FindAllString(text, -1)); n > 0 {
		t.Errorf("extraction spliced %d line(s) across columns; a sentence from one\n"+
			"column must never share a line with another:\n%s", n, firstGaps(text, 3))
	}
	// No single line may carry text from both columns: that is the splice
	// that makes a claim's quote unverifiable against its source.
	const leftOnly, rightOnly = "Sparse routing reduces", "Our evaluation harness"
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, leftOnly) && strings.Contains(line, rightOnly) {
			t.Errorf("a line carries both columns: %q", line)
		}
	}
	// Each column's lines must survive intact.
	for _, want := range []string{
		"Sparse routing reduces the compute a dense",
		"Our evaluation harness holds the data order",
		"five times fewer FLOPs than the dense",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("column text did not survive intact: %q", want)
		}
	}
}

func TestSplitSectionsKeepsBodyDespiteContentsListing(t *testing.T) {
	text, err := Extract(context.Background(), "testdata/two-column.pdf")
	if err != nil {
		t.Fatal(err)
	}
	var kept strings.Builder
	for _, s := range SplitSections("paper.pdf", text) {
		kept.WriteString(s.Body)
		kept.WriteString("\n")
	}
	body := kept.String()

	// The contents line "References  4" sits in the front matter; truncating
	// there throws away the entire paper.
	ratio := float64(len(body)) / float64(len(text))
	if ratio < 0.5 {
		t.Errorf("sectioning kept only %.1f%% of the document — the contents listing\n"+
			"was mistaken for the bibliography", ratio*100)
	}
	// Body claims must survive: they are what the grounding ledger is built from.
	flat := strings.Join(strings.Fields(body), " ")
	for _, want := range []string{
		"validation loss of 3.41",
		"five times fewer FLOPs",
		"router learns the skipping policy unaided",
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("sectioning dropped body text containing %q", want)
		}
	}
	// The real bibliography, on the last page, must still be dropped.
	if strings.Contains(flat, "Sparse routing at scale. 2025") {
		t.Error("the real references section should have been dropped")
	}
}

// firstGaps renders up to n offending lines for a readable failure message.
func firstGaps(text string, n int) string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if gapLine.MatchString(line) {
			out = append(out, "  "+line)
			if len(out) == n {
				break
			}
		}
	}
	return strings.Join(out, "\n")
}

func TestExtractDiagnosesMissingTextLayer(t *testing.T) {
	_, err := Extract(context.Background(), "testdata/no-text-layer.pdf")
	if err == nil {
		t.Fatal("a PDF with no text layer should be reported, not returned as empty text")
	}
	if !errors.Is(err, ErrNoTextLayer) {
		t.Errorf("error should be identifiable as ErrNoTextLayer, got %v", err)
	}
	// The message has to tell the reader what to do about it.
	for _, want := range []string{"no text", "scan"} {
		if !strings.Contains(strings.ToLower(err.Error()), want) {
			t.Errorf("error message should mention %q: %v", want, err)
		}
	}
}
