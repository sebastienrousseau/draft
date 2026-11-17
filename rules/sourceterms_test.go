// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package rules

import "testing"

func TestExemptSourceTerms(t *testing.T) {
	style := Style{
		BannedWords:   []string{"harness", "Robust", "leverage"},
		BannedPhrases: []string{"at its core", "in conclusion"},
	}
	// An inflection counts, a phrase may wrap across lines, case is ignored,
	// and a banned word inside a longer word is not a use of it.
	quotes := []string{"Two harnesses were tested.", "AT ITS\n  CORE the design", "unrobustness aside"}
	got, exempt := style.ExemptSourceTerms(quotes)
	if len(got.BannedWords) != 2 || got.BannedWords[0] != "Robust" || got.BannedWords[1] != "leverage" {
		t.Errorf("banned words = %v, want [Robust leverage]", got.BannedWords)
	}
	if len(got.BannedPhrases) != 1 || got.BannedPhrases[0] != "in conclusion" {
		t.Errorf("banned phrases = %v, want [in conclusion]", got.BannedPhrases)
	}
	if !exempt["harness"] || !exempt["at its core"] || len(exempt) != 2 {
		t.Errorf("exempt = %v, want harness and at its core", exempt)
	}
	if len(style.BannedWords) != 3 {
		t.Error("the receiver must not be modified")
	}
	if _, none := style.ExemptSourceTerms(nil); len(none) != 0 {
		t.Errorf("no quotes must exempt nothing, got %v", none)
	}
}
