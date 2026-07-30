// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package rules

import "testing"

func TestWordFormsCoverTheInflections(t *testing.T) {
	forms := WordForms("leverage")
	kinds := map[string]string{}
	for _, f := range forms {
		kinds[f.Kind] = f.Form
	}
	for _, want := range []string{"base", "s", "ed", "ing"} {
		if kinds[want] == "" {
			t.Errorf("no %q form produced; got %v", want, forms)
		}
	}
	if kinds["base"] != "leverage" {
		t.Errorf("base form = %q", kinds["base"])
	}
}

// Every banned term must map to a replacement, or enforceStyle silently skips
// it and the draft fails validation for a word the tool could have fixed.
func TestEveryBannedTermHasAReplacement(t *testing.T) {
	for _, w := range BannedWords {
		if StyleReplacements[w] == "" {
			t.Errorf("banned word %q has no replacement", w)
		}
	}
	for _, p := range BannedPhrases {
		if StyleReplacements[p] == "" {
			t.Errorf("banned phrase %q has no replacement", p)
		}
	}
}

// A replacement that is itself banned would loop the style pass.
func TestReplacementsAreNotThemselvesBanned(t *testing.T) {
	banned := map[string]bool{}
	for _, w := range BannedWords {
		banned[w] = true
	}
	for _, p := range BannedPhrases {
		banned[p] = true
	}
	for term, repl := range StyleReplacements {
		if banned[repl] {
			t.Errorf("%q is replaced by %q, which is itself banned", term, repl)
		}
	}
}
