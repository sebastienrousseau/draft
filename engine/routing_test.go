// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/config"
)

// The point of routing: a dozen cheap extraction calls can go to a local model
// while the single quality-critical write call goes to a session provider.
func TestNameForHonoursPerKindOverrides(t *testing.T) {
	cfg := config.Config{
		Engine:        config.EngineAuto,
		ExtractEngine: config.EngineOllama,
		WriteEngine:   "claude",
	}
	for _, tc := range []struct {
		kind Kind
		want string
	}{
		{KindExtract, config.EngineOllama},
		{KindWrite, "claude"},
		{KindEdit, config.EngineAuto}, // unset: inherits Engine
	} {
		if got := NameFor(cfg, tc.kind); got != tc.want {
			t.Errorf("NameFor(kind %v) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestNameForFallsBackToEngine(t *testing.T) {
	cfg := config.Config{Engine: config.EngineOllama}
	for _, k := range []Kind{KindExtract, KindWrite, KindEdit} {
		if got := NameFor(cfg, k); got != config.EngineOllama {
			t.Errorf("kind %v = %q, want the global default", k, got)
		}
	}
	// Whitespace is not a configuration.
	blank := config.Config{Engine: "claude", WriteEngine: "   "}
	if got := NameFor(blank, KindWrite); got != "claude" {
		t.Errorf("blank override = %q, want the global default", got)
	}
}

func TestChainForBuildsAPerKindChain(t *testing.T) {
	cfg := config.Config{Engine: config.EngineAuto, ExtractEngine: config.EngineOllama}

	extract := ChainFor(cfg, KindExtract)
	if len(extract) != 1 || extract[0].Name() != "ollama" {
		t.Fatalf("extract chain = %v, want a lone ollama", chainNames(extract))
	}
	// Chain is the write chain, kept for existing callers.
	if got, want := chainNames(Chain(cfg)), chainNames(ChainFor(cfg, KindWrite)); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Chain = %v, want the write chain %v", got, want)
	}
}

// Validate must name which setting carried the bad value: "unknown engine"
// alone leaves the user hunting through four places for the typo.
func TestValidateNamesTheOffendingSetting(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cfg     config.Config
		wantErr string
	}{
		{name: "engine", cfg: config.Config{Engine: "claud"}, wantErr: "--engine"},
		{name: "extract", cfg: config.Config{Engine: "auto", ExtractEngine: "ollamaa"}, wantErr: "--extract-engine"},
		{name: "write", cfg: config.Config{Engine: "auto", WriteEngine: "clod"}, wantErr: "--write-engine"},
		{name: "edit", cfg: config.Config{Engine: "auto", EditEngine: "nope"}, wantErr: "DRAFT_EDIT_ENGINE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.cfg)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not name %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateAcceptsPerKindOverrides(t *testing.T) {
	cfg := config.Config{
		Engine:        config.EngineAuto,
		ExtractEngine: config.EngineOllama,
		WriteEngine:   "claude",
		EditEngine:    "codex",
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("Validate = %v, want nil", err)
	}
}

func chainNames(engines []Engine) []string {
	out := make([]string, 0, len(engines))
	for _, e := range engines {
		out = append(out, e.Name())
	}
	return out
}
