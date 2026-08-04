// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package rules

import (
	"strings"
	"testing"
)

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
		{"smooth", "ly", "smoothly"},
		{"deep", "ly", "deeply"},
		{"essential", "ly", "essentially"},
		{"busy", "ly", "busily"},     // y after a consonant -> ily
		{"basic", "ly", "basically"}, // -ic -> -ally
		{"gentle", "ly", "gently"},   // -le -> -y
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
	want := map[string]string{"leverage": "base", "leverages": "s", "leveraged": "ed", "leveraging": "ing", "leveragely": "ly"}
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
	if !isVowel('A') || isVowel('z') {
		t.Error("isVowel should recognize vowels case-insensitively")
	}
}

func TestMetricForms(t *testing.T) {
	// An abbreviation and its expansion are the same metric, so a grounding
	// check must accept either without treating the swap as a conversion.
	forms := MetricForms("bpb")
	if len(forms) < 2 {
		t.Fatalf("bpb should belong to a group of equivalent forms, got %v", forms)
	}
	var found bool
	for _, f := range forms {
		if strings.Contains(strings.ToLower(f), "bits per byte") {
			found = true
		}
	}
	if !found {
		t.Errorf("bpb group should contain its expansion, got %v", forms)
	}
	// Both directions resolve to the same group.
	if got := MetricForms(forms[len(forms)-1]); len(got) != len(forms) {
		t.Errorf("expansion should map back to the same group: %v vs %v", got, forms)
	}
	// An unknown term is its own only form.
	if got := MetricForms("zzz-unknown"); len(got) != 1 || got[0] != "zzz-unknown" {
		t.Errorf("unknown term should return itself alone, got %v", got)
	}
}
