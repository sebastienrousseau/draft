// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
)

// flakyEngine fails its first n calls, then succeeds.
type flakyEngine struct {
	name     string
	failures int
	calls    int
}

func (f *flakyEngine) Name() string { return f.name }
func (f *flakyEngine) Generate(context.Context, engine.Request) (engine.Result, error) {
	f.calls++
	if f.calls <= f.failures {
		return engine.Result{}, errors.New("transient network error")
	}
	return engine.Result{Text: "ok from " + f.name}, nil
}

// A single blip on the first paper used to demote a whole queue to the local
// model for its entire life.
func TestChainRecoversAfterTheRehabilitationDelay(t *testing.T) {
	now := time.Now()
	orig := timeNow
	timeNow = func() time.Time { return now }
	defer func() { timeNow = orig }()

	primary := &flakyEngine{name: "primary", failures: 1}
	fallback := &flakyEngine{name: "fallback"}
	r := NewRunner(config.Config{}, []engine.Engine{primary, fallback}, nil)

	// First call: primary blips, the chain falls back and sticks.
	if _, err := r.generate(context.Background(), engine.Request{}); err != nil {
		t.Fatal(err)
	}
	if got := r.EngineFor(engine.KindWrite); got != "fallback" {
		t.Fatalf("after a failure the chain should be on the fallback, got %q", got)
	}

	// Before the delay elapses the chain stays put: retrying a genuinely dead
	// provider on every call would cost a failed call per job.
	now = now.Add(rehabilitationDelay - time.Minute)
	if _, err := r.generate(context.Background(), engine.Request{}); err != nil {
		t.Fatal(err)
	}
	if got := r.EngineFor(engine.KindWrite); got != "fallback" {
		t.Errorf("chain rehabilitated too early, now on %q", got)
	}

	// Once it has, the preferred engine is tried again and now works.
	now = now.Add(2 * time.Minute)
	res, err := r.generate(context.Background(), engine.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.EngineFor(engine.KindWrite); got != "primary" {
		t.Errorf("chain did not return to the primary, on %q", got)
	}
	if res.Text != "ok from primary" {
		t.Errorf("result came from %q", res.Text)
	}
}

// A chain that never failed must not be disturbed, and a still-broken primary
// must demote again rather than looping.
func TestRehabilitateIsANoOpWithoutADemotion(t *testing.T) {
	c := &chainState{engines: nil}
	if c.rehabilitate(time.Now()) {
		t.Error("an undemoted chain rehabilitated")
	}
	var nilChain *chainState
	if nilChain.rehabilitate(time.Now()) {
		t.Error("a nil chain rehabilitated")
	}
	c = &chainState{cur: 1}
	if c.rehabilitate(time.Now()) {
		t.Error("a chain with no demotion timestamp rehabilitated")
	}
}

func TestOllamaParallelismFollowsTheServerSetting(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want int
	}{
		{"", ollamaExtractConcurrency},
		{"not-a-number", ollamaExtractConcurrency},
		{"1", ollamaExtractConcurrency}, // never below the measured floor
		{"2", ollamaExtractConcurrency},
		{"6", 6},
	} {
		t.Setenv("OLLAMA_NUM_PARALLEL", tc.env)
		if got := ollamaParallelism(); got != tc.want {
			t.Errorf("OLLAMA_NUM_PARALLEL=%q: parallelism = %d, want %d", tc.env, got, tc.want)
		}
	}
}

func TestExtractConcurrencyRespectsTheServerSetting(t *testing.T) {
	ollama := &flakyEngine{name: "ollama"}
	r := NewRunner(config.Config{ExtractConcurrency: 8}, []engine.Engine{ollama}, nil)

	t.Setenv("OLLAMA_NUM_PARALLEL", "")
	if got := r.extractConcurrency(); got != ollamaExtractConcurrency {
		t.Errorf("default cap = %d, want %d", got, ollamaExtractConcurrency)
	}
	t.Setenv("OLLAMA_NUM_PARALLEL", "4")
	if got := r.extractConcurrency(); got != 4 {
		t.Errorf("with four server slots, cap = %d, want 4", got)
	}
	// The configured worker count is still the ceiling.
	t.Setenv("OLLAMA_NUM_PARALLEL", "64")
	if got := r.extractConcurrency(); got != 8 {
		t.Errorf("cap = %d, want the configured 8", got)
	}
}
