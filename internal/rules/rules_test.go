// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package rules

import "testing"

func TestInflectLike(t *testing.T) {
	cases := []struct{ word, kind, want string }{
		{"use", "s", "uses"},
		{"use", "ed", "used"},   // silent-e keeps a single d
		{"use", "ing", "using"}, // silent-e dropped before -ing
		{"unlock", "s", "unlocks"},
		{"unlock", "ed", "unlocked"},
		{"unlock", "ing", "unlocking"},
		{"harness", "s", "harnesses"}, // sibilant takes -es
		{"increase", "ing", "increasing"},
		{"use", "base", "use"},
		{"use", "unknown", "use"},
	}
	for _, c := range cases {
		if got := InflectLike(c.word, c.kind); got != c.want {
			t.Errorf("InflectLike(%q,%q) = %q, want %q", c.word, c.kind, got, c.want)
		}
	}
}

func TestWordForms(t *testing.T) {
	got := WordForms("leverage")
	want := map[string]string{"leverage": "base", "leverages": "s", "leveraged": "ed", "leveraging": "ing"}
	if len(got) != len(want) {
		t.Fatalf("leverage forms = %v, want %d entries", got, len(want))
	}
	for _, f := range got {
		if want[f.Form] != f.Kind {
			t.Errorf("form %q has kind %q, want %q", f.Form, f.Kind, want[f.Form])
		}
	}
	// A word whose inflections collide dedupes rather than repeating a form.
	for _, f := range WordForms("cutting-edge") {
		if f.Form == "" {
			t.Error("empty form produced")
		}
	}
}
