// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package engine abstracts text generation over interchangeable backends:
//
//   - Session providers (Claude, Codex, Gemini, Copilot, Cursor, Amp, Crush,
//     Goose, Grok, Qwen, ...), each driven through its own CLI in headless mode
//     using the user's already-authenticated session — no API token.
//   - Ollama, the local HTTP server, used offline or when no session CLI is
//     available.
//
// Callers depend only on the Engine interface, so the pipeline is identical
// regardless of which backend actually runs.
package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/sebastienrousseau/draft/config"
)

// Kind identifies the pipeline stage a request belongs to, letting a backend
// pick an appropriate model (Ollama uses a small model to extract and a larger
// one to write; a session provider uses one model throughout).
type Kind int

// Generation stages a Request can belong to.
const (
	KindExtract Kind = iota // per-section claim extraction
	KindWrite               // full article generation
	KindEdit                // surgical review edits
)

// Request is a single generation call.
type Request struct {
	Kind        Kind
	Prompt      string
	Temperature float64
	// NumPredict caps the output tokens for this call (0 = the engine's default).
	// The pipeline sizes it to the article's word budget so a sparse claim ledger
	// cannot pad a local model toward its token ceiling. Session providers, which
	// manage their own generation, ignore it.
	NumPredict int
	// OnChunk, if set, receives streamed text as it arrives for live preview.
	OnChunk func(string)
}

// Result is the outcome of a generation call.
type Result struct {
	Text string
	// Truncated is true when the backend stopped because it hit a length limit
	// rather than finishing, signalling the pipeline to continue generation.
	Truncated bool
}

// Engine is a text-generation backend.
type Engine interface {
	// Name is a short human label shown in the UI (e.g. "claude" or "ollama").
	Name() string
	// Generate runs one request, honouring ctx for cancellation and timeout.
	Generate(ctx context.Context, req Request) (Result, error)
}

// Chain resolves the ordered list of engines to try for a run, honouring the
// configured mode. The pipeline uses the first that succeeds and sticks with it.
//
//   - "ollama": just the local backend.
//   - a provider name: that session provider, then Ollama.
//   - "auto" (default): every installed session provider in preference order,
//     then Ollama.
//
// It deliberately does not probe the network up front: a flaky connectivity
// check must never be what downgrades an online machine to the local model.
// Instead, if a session call fails (the provider is offline or not logged in),
// the pipeline advances to the next engine in the chain and stays there.
func Chain(cfg config.Config) []Engine { return ChainFor(cfg, KindWrite) }

// ChainFor resolves the chain for one request kind, honouring that kind's
// override and falling back to cfg.Engine when it is unset.
//
// The workload is lopsided: extraction is one cheap, mechanical call per
// section, while writing is a single quality-critical call. Routing them
// separately lets a local model do the dozen extractions — free, offline, and
// no session quota — while the article itself is written by the best backend
// available.
func ChainFor(cfg config.Config, kind Kind) []Engine {
	return chainForName(cfg, engineNameFor(cfg, kind))
}

// NameFor reports which configured engine serves a request kind, before any
// chain is built. Callers use it to tell whether two kinds resolve to the same
// backend, and to display the routing.
func NameFor(cfg config.Config, kind Kind) string { return engineNameFor(cfg, kind) }

// engineNameFor is the configured engine for a kind, or the global default.
func engineNameFor(cfg config.Config, kind Kind) string {
	var override string
	switch kind {
	case KindExtract:
		override = cfg.ExtractEngine
	case KindWrite:
		override = cfg.WriteEngine
	case KindEdit:
		override = cfg.EditEngine
	}
	if strings.TrimSpace(override) != "" {
		return override
	}
	return cfg.Engine
}

func chainForName(cfg config.Config, name string) []Engine {
	ollama := NewOllama(cfg)
	switch name {
	case config.EngineOllama:
		return []Engine{ollama}
	case config.EngineAuto, "":
		var chain []Engine
		for _, p := range Providers {
			if p.Experimental && !cfg.Experimental {
				continue
			}
			if available(p.Bin) {
				if s, ok := NewSession(p.Name, cfg); ok {
					chain = append(chain, s)
				}
			}
		}
		return append(chain, ollama)
	default:
		if s, ok := NewSession(name, cfg); ok {
			return []Engine{s, ollama}
		}
		// An unknown name reaching here means Validate was not called. Fall
		// back rather than panic, but never pretend the requested engine ran:
		// Validate is what turns a typo into a clean exit.
		return []Engine{ollama}
	}
}

// Validate reports whether cfg names an engine that exists. Chain has to
// degrade to Ollama on an unknown name — it returns no error — so without this
// check `--engine claud` silently produces a local-model run that the user
// believes came from Claude. Call it once, before Chain.
func Validate(cfg config.Config) error {
	for _, s := range []struct{ setting, name string }{
		{"--engine / DRAFT_ENGINE", cfg.Engine},
		{"--extract-engine / DRAFT_EXTRACT_ENGINE", cfg.ExtractEngine},
		{"--write-engine / DRAFT_WRITE_ENGINE", cfg.WriteEngine},
		{"DRAFT_EDIT_ENGINE", cfg.EditEngine},
	} {
		if err := validateName(s.setting, s.name); err != nil {
			return err
		}
	}
	return nil
}

// validateName reports an engine name that does not exist, naming which
// setting carried it — "unknown engine" alone leaves the user hunting through
// four places for the typo.
func validateName(setting, name string) error {
	switch name {
	case config.EngineAuto, config.EngineOllama, "":
		return nil
	}
	if _, ok := LookupProvider(name); ok {
		return nil
	}
	return fmt.Errorf("%s: unknown engine %q (want %s, %s, or one of: %s)",
		setting, name, config.EngineAuto, config.EngineOllama, strings.Join(ProviderNames(), ", "))
}

// ResolveModel returns the model label the given engine will use, for display.
func ResolveModel(cfg config.Config, e Engine) string {
	if e == nil {
		return ""
	}
	if e.Name() == "ollama" {
		return cfg.OllamaModel
	}
	if cfg.Model != "" {
		return cfg.Model
	}
	if p, ok := LookupProvider(e.Name()); ok && p.DefaultModel != "" {
		return p.DefaultModel
	}
	return "session default"
}
