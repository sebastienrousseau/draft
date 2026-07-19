// Package engine abstracts text generation over two interchangeable backends:
//
//   - Claude, driven through the `claude` CLI in headless print mode, which uses
//     the user's already-authenticated session (no API token) and is preferred
//     whenever the machine is online.
//   - Ollama, the local HTTP server, used offline or when Claude is unavailable.
//
// Callers depend only on the Engine interface, so the pipeline is identical
// regardless of which backend actually runs.
package engine

import (
	"context"
	"net"
	"os/exec"
	"time"

	"github.com/sebastienrousseau/draft/internal/config"
)

// Kind identifies the pipeline stage a request belongs to, letting a backend
// pick an appropriate model (Ollama uses a small model to extract and a larger
// one to write; Claude uses one model throughout).
type Kind int

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

// Online reports whether Anthropic is reachable, using a short-timeout TCP dial
// so an offline run fails over to Ollama in well under two seconds.
func Online(ctx context.Context) bool {
	d := net.Dialer{Timeout: 1500 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", "api.anthropic.com:443")
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// ClaudeAvailable reports whether the claude CLI is installed on PATH.
func ClaudeAvailable() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

// Select resolves the primary engine and an optional fallback for a run,
// honouring the configured mode. In auto mode it prefers Claude when the
// machine is online and the CLI is present, and always keeps Ollama as the
// offline fallback.
func Select(ctx context.Context, cfg config.Config) (primary Engine, fallback Engine) {
	claude := NewClaude(cfg)
	ollama := NewOllama(cfg)
	switch cfg.Engine {
	case config.EngineClaude:
		return claude, nil
	case config.EngineOllama:
		return ollama, nil
	default: // EngineAuto
		if ClaudeAvailable() && Online(ctx) {
			return claude, ollama
		}
		return ollama, nil
	}
}
