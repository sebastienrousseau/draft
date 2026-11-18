// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"testing"
	"unicode"
)

func TestNumbersCommaHandling(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"1,000", "1000"},
		{"1,000,000.5", "1000000.5"},
		{"10,00,000", "1000000"}, // Indian grouping
		{"1,5", "1,5"},           // European decimal is not a thousands group
		{"[1,2,3]", "1,2,3"},     // a citation list is not a number
		{"12,34", "12,34"},
	}
	for _, c := range cases {
		got := Numbers(c.in)
		if len(got) != 1 || !got[c.want] {
			t.Errorf("Numbers(%q) = %v, want {%s}", c.in, got, c.want)
		}
	}
}

func TestCommaListCannotGroundJoinedNumber(t *testing.T) {
	source := "Prior work [1,2,3] reported gains of 1,5 points on average."
	for _, claim := range []string{"Prior work reported 123 results.", "Gains were 15 points."} {
		rec := Record{Claim: claim, SourceQuote: "Prior work [1,2,3] reported gains of 1,5 points on average."}
		if ok, _ := Verify(rec, source); ok {
			t.Errorf("%q verified against a quote that only has comma-separated digits", claim)
		}
	}
}

func TestNonASCIIDigitsAreChecked(t *testing.T) {
	source := "Accuracy rose to 10 percent in the second arm of the study."
	quote := "Accuracy rose to 10 percent in the second arm of the study."
	// Arabic-Indic ١٠ is 10 and must match; ٣٤ is 34 and must not.
	if ok, why := Verify(Record{Claim: "Accuracy rose to ١٠ percent.", SourceQuote: quote}, source); !ok {
		t.Fatalf("matching Arabic-Indic digits rejected: %s", why)
	}
	if ok, _ := Verify(Record{Claim: "Accuracy rose to ٣٤ percent.", SourceQuote: quote}, source); ok {
		t.Fatal("a number in Arabic-Indic digits skipped the number check")
	}
	// Devanagari nine, a full-width zero and a 32-bit mathematical digit.
	if got := asciiDigits("९０𝟗x"); got != "909x" {
		t.Fatalf("asciiDigits = %q, want 909x", got)
	}
}

// digitValue assumes each script's digits form runs of ten that start at
// zero. Hold the Unicode table to that, so a future table cannot silently
// give digits the wrong value.
func TestDigitRangesAreRunsOfTen(t *testing.T) {
	for _, rg := range digitRanges {
		if (rg[1]-rg[0]+1)%10 != 0 {
			t.Errorf("digit range %U-%U is not a whole number of runs of ten", rg[0], rg[1])
		}
		if !unicode.IsDigit(rg[0]) || unicode.IsDigit(rg[0]-1) && digitValue(rg[0]-1) != 9 {
			t.Errorf("digit range %U-%U does not start at a zero", rg[0], rg[1])
		}
	}
	for _, r := range unicode.Digit.R16 {
		if r.Stride != 1 {
			t.Errorf("R16 range %U-%U has stride %d", rune(r.Lo), rune(r.Hi), r.Stride)
		}
	}
	for _, r := range unicode.Digit.R32 {
		if r.Stride != 1 {
			t.Errorf("R32 range %U-%U has stride %d", rune(r.Lo), rune(r.Hi), r.Stride)
		}
	}
	if digitValue('a') != -1 || digitValue('0') != 0 || digitValue('9') != 9 {
		t.Fatal("digitValue is wrong on ASCII")
	}
}
