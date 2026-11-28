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
		"empty wrapper": `{"claims": []}`,
		"malformed":     "[{not json",
		"bad wrapper":   `{"claims": [{not json`,
		"prose refusal": "I cannot help with that.",
		"bare record":   `{"claim":"x"}`, // neither the wrapper nor the list
		"wrong key":     `{"records": [{"claim":"x","source_quote":"any source","type":"result","strength":"demonstrated"}]}`,
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
	pr, pd := Parse(jsonText, source)
	if len(jr) != len(tr) || jd != td || len(pr) != len(jr) || pd != jd {
		t.Fatalf("ParseJSON (%d,%d) != Parse text (%d,%d) or Parse json (%d,%d)", len(jr), jd, len(tr), td, len(pr), pd)
	}
	for i := range jr {
		if jr[i] != tr[i] || pr[i] != jr[i] {
			t.Errorf("record %d differs: json=%+v text=%+v auto=%+v", i, jr[i], tr[i], pr[i])
		}
	}
}

// hasSuffix avoids importing strings for a single call in the test file.
func hasSuffix(s, suf string) bool {
	return len(s) >= len(suf) && s[len(s)-len(suf):] == suf
}

// The schema's root is an object, {"claims": [...]}, because Claude and Codex
// require one. That wrapper must gate exactly as the bare list does, which
// cached and older Ollama output still uses, and Parse must route both to
// ParseJSON rather than reading them as CLAIM: text.
func TestParseJSONAcceptsTheClaimsWrapper(t *testing.T) {
	source := "The method used 5x fewer tokens than the baseline and tripled throughput on every device."
	list := `[{"claim": "used 5x fewer tokens", "source_quote": "used 5x fewer tokens than the baseline", "type": "result", "strength": "demonstrated"},
	          {"claim": "made up", "source_quote": "a quote that is not in the source at all", "type": "result", "strength": "demonstrated"}]`
	wrapped := `{"claims": ` + list + `}`
	lr, ld := ParseJSON(list, source)
	wr, wd := ParseJSON(wrapped, source)
	pr, pd := Parse("  "+wrapped+"\n", source)
	if len(lr) != 1 || ld != 1 {
		t.Fatalf("bare list = %d records, %d dropped; want 1, 1", len(lr), ld)
	}
	if len(wr) != len(lr) || wd != ld || len(pr) != len(lr) || pd != ld {
		t.Fatalf("wrapper (%d,%d) and Parse (%d,%d) differ from the bare list (%d,%d)", len(wr), wd, len(pr), pd, len(lr), ld)
	}
	if wr[0] != lr[0] || pr[0] != lr[0] {
		t.Errorf("wrapper record %+v differs from list record %+v", wr[0], lr[0])
	}
}
