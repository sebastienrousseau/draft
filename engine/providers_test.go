// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/sebastienrousseau/draft/config"
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
	// amp and crush are experimental; without opt-in none qualify.
	withAvailable(map[string]bool{"amp": true, "crush": true}, func() {
		if _, ok := FirstAvailableProvider(false); ok {
			t.Error("experimental providers should be skipped without opt-in")
		}
		p, ok := FirstAvailableProvider(true)
		if !ok || p.Name != "amp" { // amp precedes crush in preference order
			t.Errorf("expected amp first with opt-in, got %+v %v", p, ok)
		}
	})
	// copilot is verified (non-experimental) and qualifies by default.
	withAvailable(map[string]bool{"copilot": true}, func() {
		if p, ok := FirstAvailableProvider(false); !ok || p.Name != "copilot" {
			t.Errorf("copilot should qualify by default, got %+v %v", p, ok)
		}
	})
	withAvailable(map[string]bool{}, func() {
		if _, ok := FirstAvailableProvider(false); ok {
			t.Error("no providers installed should yield ok=false")
		}
	})
}

func TestChainAutoSkipsExperimental(t *testing.T) {
	// amp is experimental, so auto skips it unless opted in.
	withAvailable(map[string]bool{"claude": true, "amp": true}, func() {
		chain := Chain(config.Config{Engine: config.EngineAuto})
		if names(chain)[0] != "claude" || names(chain)[len(chain)-1] != "ollama" {
			t.Errorf("unexpected chain: %v", names(chain))
		}
		for _, n := range names(chain) {
			if n == "amp" {
				t.Error("experimental amp should be skipped in default auto mode")
			}
		}
		// With opt-in, amp joins the chain.
		chain = Chain(config.Config{Engine: config.EngineAuto, Experimental: true})
		got := names(chain)
		if len(got) != 3 || got[0] != "claude" || got[1] != "amp" || got[2] != "ollama" {
			t.Errorf("experimental opt-in chain wrong: %v", got)
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
	ollama := NewOllama(config.Config{OllamaModel: "gemma3:4b"})
	if got := ResolveModel(config.Config{OllamaModel: "gemma3:4b"}, ollama); got != "gemma3:4b" {
		t.Errorf("ollama model = %q, want gemma3:4b", got)
	}
}

func TestIsAvailable(t *testing.T) {
	if !IsAvailable("auto") {
		t.Error("auto should always be available")
	}
	withAvailable(map[string]bool{"claude": true, "ollama": true}, func() {
		if !IsAvailable("claude") {
			t.Error("claude should be available when installed")
		}
		if !IsAvailable("ollama") {
			t.Error("ollama should be available when installed")
		}
		if IsAvailable("codex") {
			t.Error("codex should not be available when not installed")
		}
		if IsAvailable("unknown") {
			t.Error("unknown providers should not be available")
		}
	})
}

func TestNetworkAndOllamaHelpers(t *testing.T) {
	_ = IsOnline() // exercise real call

	orig := IsOnline
	IsOnline = func() bool { return false }
	defer func() { IsOnline = orig }()

	if IsOnline() {
		t.Error("mocked IsOnline should return false")
	}

	if IsOllamaRunning("http://127.0.0.1:59999") {
		t.Error("unreachable host should return false for IsOllamaRunning")
	}
}

func TestEnsureOllamaRunning(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	if err := EnsureOllamaRunning(ts.URL); err != nil {
		t.Errorf("EnsureOllamaRunning on running host failed: %v", err)
	}

	withAvailable(map[string]bool{"ollama": false}, func() {
		if err := EnsureOllamaRunning("http://127.0.0.1:59998"); err == nil {
			t.Error("EnsureOllamaRunning without binary should return error")
		}
	})
}

func TestEnsureOllamaRunningStartsServer(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	binDir := t.TempDir()
	ollamaPath := filepath.Join(binDir, "ollama")
	if err := os.WriteFile(ollamaPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := EnsureOllamaRunning(server.URL); err != nil {
		t.Fatalf("EnsureOllamaRunning should recover after starting Ollama: %v", err)
	}
	if requests.Load() < 2 {
		t.Fatalf("expected readiness to be checked again, got %d request(s)", requests.Load())
	}
}

func names(engs []Engine) []string {
	out := make([]string, len(engs))
	for i, e := range engs {
		out[i] = e.Name()
	}
	return out
}
