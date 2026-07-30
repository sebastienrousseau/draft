// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"strings"
	"testing"
)

// Found by FuzzParse. Comparison normalises with strings.ToLower, which maps
// every invalid UTF-8 byte to U+FFFD — so two DIFFERENT invalid byte sequences
// normalised to the same string and a fabricated quote matched a source it
// does not occur in. That is a bypass of the one invariant the whole ledger
// rests on.
func TestVerifyRejectsInvalidUTF8Quote(t *testing.T) {
	// Neither byte sequence appears in the other, but both collapse to a run
	// of U+FFFD once lowercased.
	quote := strings.Repeat("\xe2", 11) + "0"
	source := strings.Repeat("\x88", 11) + "0"

	ok, why := Verify(Record{Claim: "a claim", SourceQuote: quote, Type: "result", Strength: "demonstrated"}, source)
	if ok {
		t.Fatalf("an ungrounded quote passed verification (reason %q)", why)
	}
	if !strings.Contains(why, "UTF-8") {
		t.Errorf("reason = %q, want it to name the encoding problem", why)
	}

	// And the same through Parse, which is what the pipeline actually calls.
	block := "CLAIM: a claim\nSOURCE_QUOTE: \"" + quote + "\"\nTYPE: result\nSTRENGTH: demonstrated\n---"
	records, dropped := Parse(block, source)
	if len(records) != 0 {
		t.Errorf("Parse kept %d ungrounded record(s)", len(records))
	}
	if dropped != 1 {
		t.Errorf("dropped = %d, want 1", dropped)
	}
}

// A quote made of literal replacement characters must not match a source whose
// invalid bytes normalise into them.
func TestVerifyRejectsReplacementCharacterQuote(t *testing.T) {
	quote := strings.Repeat("�", 12)
	source := strings.Repeat("\x88", 12)

	if ok, why := Verify(Record{Claim: "a claim", SourceQuote: quote}, source); ok {
		t.Fatalf("a replacement-character quote passed verification (reason %q)", why)
	}
}

// The guard must not reject legitimate non-ASCII quotes.
func TestVerifyAcceptsValidNonASCIIQuote(t *testing.T) {
	source := "L'étude a mesuré une réduction de 5x des opérations en virgule flottante."
	quote := "une réduction de 5x des opérations"

	ok, why := Verify(Record{
		Claim:       "The study measured a 5x reduction",
		SourceQuote: quote,
		Type:        "result",
		Strength:    "demonstrated",
	}, source)
	if !ok {
		t.Errorf("a valid accented quote was rejected: %s", why)
	}
}

func TestVerifyAcceptsCJKQuote(t *testing.T) {
	source := "この手法は演算量を5分の1に削減しました。"
	quote := "演算量を5分の1に削減しました"

	if ok, why := Verify(Record{
		Claim:       "5分の1に削減",
		SourceQuote: quote,
		Type:        "result",
		Strength:    "demonstrated",
	}, source); !ok {
		t.Errorf("a valid CJK quote was rejected: %s", why)
	}
}
