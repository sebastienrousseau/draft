// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import "testing"

func TestParseJSONVerifiesAndDrops(t *testing.T) {
	source := "The system reached a score of 0.82 on the test set. It used 5x fewer tokens than the baseline."
	// Two quotes are verbatim; the third is not in the source.
	text := `[
      {"claim": "It scored 0.82", "source_quote": "reached a score of 0.82 on the test set", "type": "metric", "strength": "demonstrated"},
      {"claim": "Fewer tokens", "source_quote": "used 5x fewer tokens than the baseline", "type": "result", "strength": "demonstrated"},
      {"claim": "Invented", "source_quote": "a phrase that never appears", "type": "result", "strength": "demonstrated"}
    ]`
	records, dropped := ParseJSON(text, source)
	if len(records) != 2 || dropped != 1 {
		t.Fatalf("ParseJSON = %d records, %d dropped; want 2, 1", len(records), dropped)
	}
	if records[0].Type != "metric" || records[1].Strength != "demonstrated" {
		t.Errorf("fields not carried through: %+v", records)
	}
}

func TestParseJSONRejectsUngroundedNumber(t *testing.T) {
	source := "reached a val_bpb of 0.82 on the held-out set"
	// The quote is verbatim but the claim asserts a number (0.99) the quote
	// does not contain: the same number gate the text parser applies.
	text := `[{"claim": "reached 0.99 accuracy", "source_quote": "reached a val_bpb of 0.82 on the held-out set", "type": "metric", "strength": "demonstrated"}]`
	records, dropped := ParseJSON(text, source)
	if len(records) != 0 || dropped != 1 {
		t.Errorf("ParseJSON = %d records, %d dropped; want 0, 1 (ungrounded number)", len(records), dropped)
	}
}

func TestParseJSONRepairsATruncatedQuote(t *testing.T) {
	src := "Background text here. Regev gives a polynomial-time quantum algorithm for DSP, but it relies on a subset sum oracle to erase input bits. The next sentence follows! Then a question? End"
	text := `[{"claim": "Regev gives a polynomial-time quantum algorithm for DSP that relies on a subset sum oracle",
      "source_quote": "Regev gives a polynomial-time quantum algorithm for DSP, but",
      "type": "mechanism", "strength": "demonstrated"}]`
	records, dropped := ParseJSON(text, src)
	if len(records) != 1 || dropped != 0 || !hasSuffix(records[0].SourceQuote, "input bits.") {
		t.Errorf("ParseJSON after repair = %+v dropped %d", records, dropped)
	}
}

func TestParseJSONEmptyAndMalformed(t *testing.T) {
	for name, in := range map[string]string{
		"empty string":  "",
		"whitespace":    "   \n ",
		"empty array":   "[]",
		"malformed":     "[{not json",
		"prose refusal": "I cannot help with that.",
		"object":        `{"claim":"x"}`, // not the array shape
	} {
		if r, d := ParseJSON(in, "any source"); len(r) != 0 || d != 0 {
			t.Errorf("%s: ParseJSON = %d records, %d dropped; want 0, 0", name, len(r), d)
		}
	}
}

// ParseJSON must gate exactly as the text Parse does: the same source and
// equivalent records in either wire format yield the same verified result.
func TestParseJSONMatchesTextParse(t *testing.T) {
	source := "The method used 5x fewer tokens than the baseline and tripled throughput on every device."
	jsonText := `[
      {"claim": "used 5x fewer tokens", "source_quote": "used 5x fewer tokens than the baseline", "type": "result", "strength": "demonstrated"},
      {"claim": "tripled throughput", "source_quote": "tripled throughput on every device", "type": "result", "strength": "demonstrated"}
    ]`
	textForm := "CLAIM: used 5x fewer tokens\nSOURCE_QUOTE: \"used 5x fewer tokens than the baseline\"\nTYPE: result\nSTRENGTH: demonstrated\n---" +
		"\nCLAIM: tripled throughput\nSOURCE_QUOTE: \"tripled throughput on every device\"\nTYPE: result\nSTRENGTH: demonstrated\n---"

	jr, jd := ParseJSON(jsonText, source)
	tr, td := Parse(textForm, source)
	if len(jr) != len(tr) || jd != td {
		t.Fatalf("ParseJSON (%d,%d) != Parse (%d,%d)", len(jr), jd, len(tr), td)
	}
	for i := range jr {
		if jr[i] != tr[i] {
			t.Errorf("record %d differs: json=%+v text=%+v", i, jr[i], tr[i])
		}
	}
}

// hasSuffix avoids importing strings for a single call in the test file.
func hasSuffix(s, suf string) bool {
	return len(s) >= len(suf) && s[len(s)-len(suf):] == suf
}
