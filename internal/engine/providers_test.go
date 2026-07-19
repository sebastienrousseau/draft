package engine

import (
	"testing"

	"github.com/sebastienrousseau/draft/internal/config"
)

func withAvailable(installed map[string]bool, fn func()) {
	orig := available
	available = func(bin string) bool { return installed[bin] }
	defer func() { available = orig }()
	fn()
}

func TestLookupProvider(t *testing.T) {
	if p, ok := LookupProvider("claude"); !ok || p.Bin != "claude" {
		t.Errorf("claude lookup failed: %+v %v", p, ok)
	}
	if _, ok := LookupProvider("missing"); ok {
		t.Error("missing provider should not resolve")
	}
}

func TestProviderNames(t *testing.T) {
	names := ProviderNames()
	if len(names) != len(Providers) || names[0] != "claude" {
		t.Errorf("unexpected provider names: %v", names)
	}
}

func TestFirstAvailableProvider(t *testing.T) {
	withAvailable(map[string]bool{"codex": true, "amp": true}, func() {
		p, ok := FirstAvailableProvider()
		if !ok || p.Name != "codex" { // codex precedes amp in preference order
			t.Errorf("expected codex first, got %+v %v", p, ok)
		}
	})
	withAvailable(map[string]bool{}, func() {
		if _, ok := FirstAvailableProvider(); ok {
			t.Error("no providers installed should yield ok=false")
		}
	})
}

func TestChainAuto(t *testing.T) {
	withAvailable(map[string]bool{"claude": true, "amp": true}, func() {
		chain := Chain(config.Config{Engine: config.EngineAuto})
		// claude, amp (in order), then ollama.
		if len(chain) != 3 {
			t.Fatalf("expected 3 engines, got %d", len(chain))
		}
		if chain[0].Name() != "claude" || chain[1].Name() != "amp" || chain[2].Name() != "ollama" {
			t.Errorf("unexpected chain order: %s, %s, %s", chain[0].Name(), chain[1].Name(), chain[2].Name())
		}
	})
}

func TestChainAutoNoProviders(t *testing.T) {
	withAvailable(map[string]bool{}, func() {
		chain := Chain(config.Config{Engine: config.EngineAuto})
		if len(chain) != 1 || chain[0].Name() != "ollama" {
			t.Errorf("expected ollama-only chain, got %d engines", len(chain))
		}
	})
}

func TestChainForcedProvider(t *testing.T) {
	chain := Chain(config.Config{Engine: "grok"})
	if len(chain) != 2 || chain[0].Name() != "grok" || chain[1].Name() != "ollama" {
		t.Errorf("forced provider chain wrong: %v", names(chain))
	}
}

func TestChainForcedUnknownProvider(t *testing.T) {
	chain := Chain(config.Config{Engine: "does-not-exist"})
	if len(chain) != 1 || chain[0].Name() != "ollama" {
		t.Errorf("unknown provider should fall back to ollama-only, got %v", names(chain))
	}
}

func TestChainOllama(t *testing.T) {
	chain := Chain(config.Config{Engine: config.EngineOllama})
	if len(chain) != 1 || chain[0].Name() != "ollama" {
		t.Errorf("ollama mode should be ollama-only, got %v", names(chain))
	}
}

func TestResolveModel(t *testing.T) {
	if got := ResolveModel(config.Config{}, nil); got != "" {
		t.Errorf("nil engine should give empty model, got %q", got)
	}
	claude, _ := NewSession("claude", config.Config{})
	if got := ResolveModel(config.Config{}, claude); got != "sonnet" {
		t.Errorf("claude default model = %q, want sonnet", got)
	}
	if got := ResolveModel(config.Config{Model: "opus"}, claude); got != "opus" {
		t.Errorf("override model = %q, want opus", got)
	}
	crush, _ := NewSession("crush", config.Config{})
	if got := ResolveModel(config.Config{}, crush); got != "session default" {
		t.Errorf("crush (no default) = %q, want 'session default'", got)
	}
	ollama := NewOllama(config.Config{OllamaModel: "qwen3:4b"})
	if got := ResolveModel(config.Config{OllamaModel: "qwen3:4b"}, ollama); got != "qwen3:4b" {
		t.Errorf("ollama model = %q, want qwen3:4b", got)
	}
}

func names(engs []Engine) []string {
	out := make([]string, len(engs))
	for i, e := range engs {
		out[i] = e.Name()
	}
	return out
}
