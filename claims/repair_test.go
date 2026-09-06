// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"strings"
	"testing"
)

// Every rule in Normalise answers a dropped claim measured on 2026-09-06,
// where "quote not found in source" was 552 of 953 drops and the quotes were
// the source's own words with a literal "\n" where a line broke, a hyphen the
// model dropped, a ligature the PDF rendered as one glyph, or a non-breaking
// space. None of those is a different fact, and a quote that changes a word
// must still fail.
func TestNormaliseTolerantOfRenderingNotOfWords(t *testing.T) {
	matches := []struct{ name, src, quote string }{
		{"literal newline escape", "the value was high", `the value\nwas high`},
		{"line-break hyphen", "the cryp-\ntography step", "the cryptography step"},
		{"model dropped a hyphen", "a polynomial-time algorithm", "a polynomialtime algorithm"},
		{"ligature", "the ﬁnal result", "the final result"},
		{"smart quotes", "it was “consistent”", `it was "consistent"`},
		{"non-breaking space", "a non breaking gap", "a non breaking gap"},
		{"soft hyphen and zero-width", "docu\u00adment\u200bed", "documented"},
		{"en and em dashes vanish", "score — up, cost – down", "score up, cost down"},
	}
	for _, tc := range matches {
		if !quoteInSource(tc.quote, tc.src) {
			t.Errorf("%s: quote %q should be found in %q after normalisation", tc.name, tc.quote, tc.src)
		}
	}
	misses := []struct{ name, src, quote string }{
		{"a changed word", "the quick brown fox", "the slow brown fox"},
		{"a missing word", "the quick brown fox", "the quick fox"},
		{"reordered words", "alpha then beta", "beta then alpha"},
		{"an added negation", "the result was stable", "the result was not stable"},
	}
	for _, tc := range misses {
		if quoteInSource(tc.quote, tc.src) {
			t.Errorf("%s: quote %q must NOT match %q", tc.name, tc.quote, tc.src)
		}
	}
	if Normalise(`Ab\nC`) != "ab c" {
		t.Errorf("Normalise escape = %q", Normalise(`Ab\nC`))
	}
}

func TestRepairExtendsACutQuoteFromTheSource(t *testing.T) {
	src := "Background text here. Regev gives a polynomial-time quantum algorithm for DSP, but it relies on a subset sum oracle to erase input bits. The next sentence follows! Then a question? End"
	full := "Regev gives a polynomial-time quantum algorithm for DSP, but it relies on a subset sum oracle to erase input bits."
	cases := map[string]struct{ in, want string }{
		"dangling conjunction":  {"Regev gives a polynomial-time quantum algorithm for DSP, but", full},
		"too short":             {"Regev gives", full},
		"exclamation boundary":  {"The next", "The next sentence follows!"},
		"question boundary":     {"Then a", "Then a question?"},
		"case-insensitive find": {"regev GIVES a polynomial-time quantum algorithm for DSP, but", full},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := repair(Record{SourceQuote: tc.in}, src).SourceQuote; got != tc.want {
				t.Errorf("repair(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	// A period inside a number is not a sentence end: repair skips "3.5" and
	// extends to the real terminator.
	if got := repair(Record{SourceQuote: "see 3."}, "see 3.5 later and then. done").SourceQuote; got != "see 3.5 later and then." {
		t.Errorf("repair should skip the decimal point, got %q", got)
	}

	// A repaired quote is verbatim by construction, so Parse now keeps what it
	// used to drop as a truncated fragment.
	block := "CLAIM: Regev gives a polynomial-time quantum algorithm for DSP that relies on a subset sum oracle\nSOURCE_QUOTE: \"Regev gives a polynomial-time quantum algorithm for DSP, but\"\nTYPE: mechanism\nSTRENGTH: demonstrated\n---"
	records, dropped := Parse(block, src)
	if len(records) != 1 || dropped != 0 || !strings.HasSuffix(records[0].SourceQuote, "input bits.") {
		t.Errorf("Parse after repair = %+v dropped %d", records, dropped)
	}
}

// repair must never reach across a sentence boundary to pull in a number that
// would validate a fabricated claim. This is the fabrication guard for the
// extension: the quote anchors in one sentence, the invented figure lives in
// the next, and the record still drops.
func TestRepairCannotCrossASentenceToValidateAFabrication(t *testing.T) {
	src := "The score was low and disappointing. The unrelated baseline hit 99 percent elsewhere."
	block := "CLAIM: The score reached 99 percent\nSOURCE_QUOTE: \"The score was low and\"\nTYPE: result\nSTRENGTH: demonstrated\n---"
	records, dropped := Parse(block, src)
	if len(records) != 0 || dropped != 1 {
		t.Errorf("a number from the next sentence must not validate the claim: records=%+v dropped=%d", records, dropped)
	}
	// The quote itself was extendable, so the drop is the number check, not a
	// failure to find the quote.
	if got := repair(Record{SourceQuote: "The score was low and"}, src).SourceQuote; got != "The score was low and disappointing." {
		t.Errorf("repair stopped at the wrong place: %q", got)
	}
}

func TestRepairLeavesWhatItCannotProve(t *testing.T) {
	longSrc := "A short source. " + strings.Repeat("word ", 80) + "end without terminator"
	unchanged := []struct{ name, quote, src string }{
		{"not in source", "Nothing like this, and", longSrc},
		{"already whole", "A short source.", longSrc},
		{"empty", "", longSrc},
		{"no sentence end within reach", "word word word word, and", longSrc},
		{"longer than the cap", strings.Repeat("x", maxRepairExtension+1) + " and", longSrc},
		{"end of source with no terminator", "end without", longSrc},
		{"case fold changes byte length", "the İ and", "the İ and then. Done"},
	}
	for _, tc := range unchanged {
		t.Run(tc.name, func(t *testing.T) {
			if got := repair(Record{SourceQuote: tc.quote}, tc.src).SourceQuote; got != tc.quote {
				t.Errorf("repair changed %q to %q", tc.quote, got)
			}
		})
	}

	// The extension is bounded. "start, and" is 10 characters, so the search
	// begins at offset 10; a terminator within maxRepairExtension of it is
	// reached, one past it is not.
	within := "start, and " + strings.Repeat("y", maxRepairExtension-2) + "."
	if got := repair(Record{SourceQuote: "start, and"}, within).SourceQuote; !strings.HasSuffix(got, ".") {
		t.Errorf("a terminator within the cap should be reached, got a %d-char result", len(got))
	}
	past := "start, and " + strings.Repeat("y", maxRepairExtension) + "."
	if got := repair(Record{SourceQuote: "start, and"}, past).SourceQuote; got != "start, and" {
		t.Errorf("a terminator past the cap must not be reached, got a %d-char result", len(got))
	}
}

// An invalid label is still a dropped record: TYPE and STRENGTH are a closed
// vocabulary, and a value outside it means the model improvised the schema.
// repair rehabilitates a quote, never a label.
func TestRepairDoesNotRescueAnInvalidLabel(t *testing.T) {
	src := "The system reached a score of 0.82 on the test set."
	block := "CLAIM: The system reached a score of 0.82 on the test set\nSOURCE_QUOTE: \"The system reached a score of 0.82 on the test set.\"\nTYPE: metric\nSTRENGTH: none\n---"
	if records, dropped := Parse(block, src); len(records) != 0 || dropped != 1 {
		t.Errorf("an invalid STRENGTH must still drop the record: records=%+v dropped=%d", records, dropped)
	}
}
