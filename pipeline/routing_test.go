// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
)

// recorder is an engine that records which kinds it was asked to serve.
type recorder struct {
	name string
	mu   sync.Mutex
	got  []engine.Kind
	err  error
}

func (e *recorder) Name() string { return e.name }

func (e *recorder) Generate(_ context.Context, req engine.Request) (engine.Result, error) {
	e.mu.Lock()
	e.got = append(e.got, req.Kind)
	e.mu.Unlock()
	if e.err != nil {
		return engine.Result{}, e.err
	}
	return engine.Result{Text: "ok"}, nil
}

func (e *recorder) kinds() []engine.Kind {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]engine.Kind(nil), e.got...)
}

// routedRunner wires explicit chains, standing in for what NewRoutedRunner
// resolves from configuration.
func routedRunner(extract, write engine.Engine) *Runner {
	r := NewRunner(config.Config{}, []engine.Engine{write}, nil)
	r.chains[engine.KindExtract] = &chainState{engines: []engine.Engine{extract}}
	return r
}

// The whole point of routing: a dozen cheap extraction calls go to one backend
// while the single quality-critical write call goes to another.
func TestRoutingSendsEachKindToItsOwnEngine(t *testing.T) {
	extractor := &recorder{name: "ollama"}
	writer := &recorder{name: "claude"}
	r := routedRunner(extractor, writer)
	r.done = context.Background().Done()

	for _, k := range []engine.Kind{engine.KindExtract, engine.KindExtract, engine.KindWrite} {
		if _, err := r.generate(context.Background(), engine.Request{Kind: k}); err != nil {
			t.Fatalf("kind %v: %v", k, err)
		}
	}

	if got := extractor.kinds(); len(got) != 2 {
		t.Errorf("extractor served %d call(s), want 2", len(got))
	}
	if got := writer.kinds(); len(got) != 1 || got[0] != engine.KindWrite {
		t.Errorf("writer served %v, want one KindWrite", got)
	}
}

// Extraction failing over must not drag writing down with it: the chains are
// separately sticky.
func TestExtractionFalloverDoesNotMoveTheWriteCursor(t *testing.T) {
	deadExtractor := &recorder{name: "dead", err: errors.New("not logged in")}
	backupExtractor := &recorder{name: "ollama"}
	writer := &recorder{name: "claude"}

	r := NewRunner(config.Config{}, []engine.Engine{writer}, nil)
	r.chains[engine.KindExtract] = &chainState{engines: []engine.Engine{deadExtractor, backupExtractor}}
	r.done = context.Background().Done()

	if _, err := r.generate(context.Background(), engine.Request{Kind: engine.KindExtract}); err != nil {
		t.Fatalf("extraction should have fallen over: %v", err)
	}
	if r.chainFor(engine.KindWrite).cur != 0 {
		t.Error("the write cursor advanced because extraction failed over")
	}
	if _, err := r.generate(context.Background(), engine.Request{Kind: engine.KindWrite}); err != nil {
		t.Fatalf("writing should still work: %v", err)
	}
	if got := writer.kinds(); len(got) != 1 {
		t.Errorf("writer served %v, want exactly one call", got)
	}
}

// NewRunner keeps today's behaviour exactly: one chain, one shared cursor, so a
// fallback found while extracting is not rediscovered when writing.
func TestNewRunnerSharesOneCursorAcrossKinds(t *testing.T) {
	dead := &recorder{name: "dead", err: errors.New("offline")}
	alive := &recorder{name: "ollama"}
	r := NewRunner(config.Config{}, []engine.Engine{dead, alive}, nil)
	r.done = context.Background().Done()

	if _, err := r.generate(context.Background(), engine.Request{Kind: engine.KindExtract}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.generate(context.Background(), engine.Request{Kind: engine.KindWrite}); err != nil {
		t.Fatal(err)
	}
	// The dead engine is tried once in total, not once per kind.
	if got := dead.kinds(); len(got) != 1 {
		t.Errorf("dead engine tried %d times, want 1 (the cursor must be shared)", len(got))
	}
}

func TestUnknownRequestKindUsesWriteChain(t *testing.T) {
	r := NewRunner(config.Config{}, []engine.Engine{&recorder{name: "writer"}}, nil)
	if r.chainFor(engine.Kind(99)) != r.chainFor(engine.KindWrite) {
		t.Fatal("an unknown request kind did not fall back to the write chain")
	}
}

// A Runner reused across a queue keeps the backend it settled on, so a dead
// provider is tried once for the whole queue rather than once per paper.
func TestFallbackIsStickyAcrossAQueue(t *testing.T) {
	cfg := testConfig(t)
	dead := &fakeEngine{name: "dead", failAll: errors.New("offline")}
	alive := okEngine("ollama")

	events := make(chan Event, 4096)
	runner := NewRunner(cfg, []engine.Engine{dead, alive}, events)

	for i := 0; i < 3; i++ {
		runner.Run(context.Background(), Job{Sources: []string{writeSource(t)}})
	}
	close(events)

	var failures int
	for e := range events {
		if w, ok := e.(WarnEvent); ok && strings.Contains(string(w), "dead failed") {
			failures++
		}
	}
	if failures != 1 {
		t.Errorf("the dead provider was reported %d times across 3 jobs; want 1", failures)
	}
}

// NewRoutedRunner resolves chains from configuration, and kinds pointed at the
// same engine share one chain so a fallback is not rediscovered.
func TestNewRoutedRunnerSharesChainsByEngineName(t *testing.T) {
	r := NewRoutedRunner(config.Config{
		Engine:      config.EngineOllama,
		WriteEngine: config.EngineOllama, // same as the default
	}, nil)
	if r.chainFor(engine.KindExtract) != r.chainFor(engine.KindWrite) {
		t.Error("kinds resolving to the same engine should share one chain")
	}

	split := NewRoutedRunner(config.Config{
		Engine:        config.EngineOllama,
		ExtractEngine: config.EngineOllama,
		WriteEngine:   "claude",
	}, nil)
	if split.chainFor(engine.KindExtract) == split.chainFor(engine.KindWrite) {
		t.Error("kinds resolving to different engines must not share a chain")
	}
	if got := split.EngineFor(engine.KindExtract); got != "ollama" {
		t.Errorf("extract engine = %q, want ollama", got)
	}
}

// DoneEvent reports the engine that produced the article, which under routing
// is the writer rather than whichever backend most recently ran.
func TestDoneEventReportsTheWriter(t *testing.T) {
	cfg := testConfig(t)
	writer := okEngine("the-writer")
	extractor := okEngine("the-extractor")

	events := make(chan Event, 4096)
	r := NewRunner(cfg, []engine.Engine{writer}, events)
	r.chains[engine.KindExtract] = &chainState{engines: []engine.Engine{extractor}}
	r.Run(context.Background(), Job{Sources: []string{writeSource(t)}})
	close(events)

	var done DoneEvent
	for e := range events {
		if d, ok := e.(DoneEvent); ok {
			done = d
		}
	}
	if done.Engine != "the-writer" {
		t.Errorf("DoneEvent.Engine = %q, want the writing engine", done.Engine)
	}
}
