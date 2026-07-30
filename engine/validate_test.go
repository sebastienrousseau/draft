// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/config"
)

// Chain has to degrade to Ollama when it cannot resolve an engine name, so a
// typo used to produce a silent local-model run. Validate is what turns that
// into a clean refusal.
func TestValidateRejectsUnknownEngine(t *testing.T) {
	err := Validate(config.Config{Engine: "claud"})
	if err == nil {
		t.Fatal("expected an error for a misspelled provider name")
	}
	if !strings.Contains(err.Error(), "claud") {
		t.Errorf("error should name the offending value, got %q", err)
	}
	// The message must list the alternatives, or the user cannot self-correct.
	for _, want := range []string{"auto", "ollama", "claude"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got %q", want, err)
		}
	}
}

func TestValidateAcceptsEveryReachableEngine(t *testing.T) {
	accepted := []string{"", config.EngineAuto, config.EngineOllama}
	accepted = append(accepted, ProviderNames()...)
	for _, name := range accepted {
		if err := Validate(config.Config{Engine: name}); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", name, err)
		}
	}
}

// An unknown name still has to fall back rather than panic, because Chain is
// reachable without Validate from library callers.
func TestChainFallsBackOnUnknownEngine(t *testing.T) {
	chain := Chain(config.Config{Engine: "nope"})
	if len(chain) != 1 || chain[0].Name() != "ollama" {
		t.Fatalf("expected a lone ollama fallback, got %d engines", len(chain))
	}
}

// LookupProvider scans Providers rather than an index frozen at init, so a
// caller that extends the registry is not silently ignored.
func TestLookupProviderSeesRegistryMutation(t *testing.T) {
	original := Providers
	t.Cleanup(func() { Providers = original })

	Providers = append(append([]Provider{}, original...), Provider{Name: "housecat", Bin: "housecat"})
	if _, ok := LookupProvider("housecat"); !ok {
		t.Error("LookupProvider did not see a provider appended to the registry")
	}
	if err := Validate(config.Config{Engine: "housecat"}); err != nil {
		t.Errorf("Validate should accept a registered provider, got %v", err)
	}
}

func TestIsLengthStop(t *testing.T) {
	for _, tc := range []struct {
		reason string
		want   bool
	}{
		{"max_tokens", true},
		{"MAX_TOKENS", true},
		{" max_output_tokens ", true},
		{"length", true},
		{"end_turn", false},
		{"stop_sequence", false},
		{"", false},
	} {
		if got := isLengthStop(tc.reason); got != tc.want {
			t.Errorf("isLengthStop(%q) = %v, want %v", tc.reason, got, tc.want)
		}
	}
}
