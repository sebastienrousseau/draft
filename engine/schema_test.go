// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/sebastienrousseau/draft/config"
)

type plainEngine struct{}

func (plainEngine) Name() string { return "plain" }
func (plainEngine) Generate(context.Context, Request) (Result, error) {
	return Result{}, nil
}

// Only an engine that actually applies Request.Schema may be sent one: the
// pipeline asks such an engine for JSON, and an engine that ignored the
// schema would get a JSON prompt with nothing holding it to the shape.
func TestSupportsSchemaNeedsTheInterface(t *testing.T) {
	if SupportsSchema(plainEngine{}) {
		t.Error("an engine without SupportsSchema must not be treated as schema-capable")
	}
	if SupportsSchema(nil) {
		t.Error("a nil engine must not be treated as schema-capable")
	}
}

// Structured output depends on the model and runtime: qwen3.5 under Ollama
// 0.40's nvfp4 runtime answers any format with 501 "structured output is
// unavailable". The probe must read that as "no" and a 200 as "yes", ask
// the server only once, and send a format so it is testing the real thing.
func TestOllamaProbesForStructuredOutput(t *testing.T) {
	withEnsureStub(t)
	for _, tc := range []struct {
		name   string
		status int
		want   bool
	}{
		{"supported", http.StatusOK, true},
		{"unavailable", http.StatusNotImplemented, false},
		{"bad request", http.StatusBadRequest, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			o := ollamaFor(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["format"] == nil {
					t.Error("the probe must send a format")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"error":"structured output is unavailable"}`))
			})
			for i := 0; i < 3; i++ {
				if got := SupportsSchema(o); got != tc.want {
					t.Fatalf("SupportsSchema = %v, want %v", got, tc.want)
				}
			}
			if n := calls.Load(); n != 1 {
				t.Errorf("probed the server %d times, want once", n)
			}
		})
	}
}

// An unreachable server is not schema-capable: the text prompt is the path
// that worked before, so doubt resolves to it.
func TestOllamaProbeFailsClosedWhenUnreachable(t *testing.T) {
	withEnsureStub(t)
	o := NewOllama(configWithHost("http://127.0.0.1:1"))
	if SupportsSchema(o) {
		t.Error("an unreachable server must not be treated as schema-capable")
	}
}

// withEnsureStub keeps the probe from starting a real ollama serve.
func withEnsureStub(t *testing.T) {
	t.Helper()
	orig := ensureRunning
	ensureRunning = func(string) error { return nil }
	t.Cleanup(func() { ensureRunning = orig })
}

func configWithHost(host string) config.Config {
	return config.Config{OllamaHost: host, ExtractModel: "m", OllamaModel: "m", EditModel: "m"}
}
