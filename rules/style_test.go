// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"testing"
)

func writeStyle(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "style.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaultStyleMatchesTheConstants(t *testing.T) {
	d := DefaultStyle()
	if d.MinWords != MinWords || d.MaxWords != MaxWords || d.English != "British English" {
		t.Errorf("default style = %+v", d)
	}
	if len(d.BannedWords) != len(BannedWords) || len(d.BannedPhrases) != len(BannedPhrases) {
		t.Error("default style must carry the built-in vocabulary")
	}
	// The copies are independent: mutating the style must not touch the globals.
	d.BannedWords[0] = "MUTATED"
	if BannedWords[0] == "MUTATED" {
		t.Error("DefaultStyle shares its slice with the package global")
	}
}

func TestLoadStyleMergesOntoDefault(t *testing.T) {
	// A file that sets only one field keeps the defaults for the rest.
	s, err := LoadStyle(writeStyle(t, `{"max_words": 1200}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.MaxWords != 1200 || s.MinWords != MinWords || len(s.BannedWords) != len(BannedWords) {
		t.Errorf("partial style = %+v", s)
	}

	// Replacement sets the list outright; "also_" extends it.
	s, err = LoadStyle(writeStyle(t, `{"banned_words": ["foo"], "also_banned_words": ["Bar", "foo", " "], "english": "American English"}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.English != "American English" {
		t.Errorf("english = %q", s.English)
	}
	// foo (replacement) + bar (addition), lower-cased, de-duplicated, sorted.
	if len(s.BannedWords) != 2 || s.BannedWords[0] != "bar" || s.BannedWords[1] != "foo" {
		t.Errorf("merged banned words = %v", s.BannedWords)
	}

	// also_ alone extends the default set.
	s, err = LoadStyle(writeStyle(t, `{"also_banned_phrases": ["at the end of the day"]}`))
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, p := range s.BannedPhrases {
		if p == "at the end of the day" {
			found = true
		}
	}
	if !found || len(s.BannedPhrases) <= len(BannedPhrases) {
		t.Errorf("also_banned_phrases did not extend the default: %d phrases", len(s.BannedPhrases))
	}
}

func TestLoadStyleRejects(t *testing.T) {
	cases := map[string]string{
		"unknown field":  `{"colour": "british"}`,
		"inverted band":  `{"min_words": 900, "max_words": 100}`,
		"zero min":       `{"min_words": 0}`,
		"malformed json": `{"max_words":`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadStyle(writeStyle(t, content)); err == nil {
				t.Errorf("expected an error for %s", name)
			}
		})
	}
	if _, err := LoadStyle(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Error("a missing file must error")
	}
}

func TestStyleOrDefaultAndBannedForms(t *testing.T) {
	if (Style{}).OrDefault().MaxWords != MaxWords {
		t.Error("a zero style must become the default")
	}
	custom := Style{MinWords: 1, MaxWords: 2, BannedWords: []string{"x"}}
	if custom.OrDefault().MaxWords != 2 {
		t.Error("a set style must be left alone")
	}
	forms := Style{BannedWords: []string{"leverage"}}.BannedWordForms()
	var hasLeverages bool
	for _, f := range forms {
		if f == "leverages" {
			hasLeverages = true
		}
	}
	if !hasLeverages {
		t.Errorf("banned word forms should include inflections: %v", forms)
	}
	if len(Style{}.BannedWordForms()) != 0 {
		t.Error("no banned words means no forms")
	}
}
