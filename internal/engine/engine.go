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

	"github.com/sebastienrousseau/draft/internal/config"
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
func Chain(cfg config.Config) []Engine {
	ollama := NewOllama(cfg)
	switch cfg.Engine {
	case config.EngineOllama:
		return []Engine{ollama}
	case config.EngineAuto, "":
		var chain []Engine
		for _, p := range Providers {
			if available(p.Bin) {
				if s, ok := NewSession(p.Name, cfg); ok {
					chain = append(chain, s)
				}
			}
		}
		return append(chain, ollama)
	default:
		if s, ok := NewSession(cfg.Engine, cfg); ok {
			return []Engine{s, ollama}
		}
		return []Engine{ollama}
	}
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
