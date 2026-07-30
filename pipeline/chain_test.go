// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
)

// A backend that genuinely fails must be fallen back from — the fallback is
// what makes an offline machine keep working — and the survivor is stuck with
// for the rest of the run.
func TestGenerateFallsOverAndSticks(t *testing.T) {
	var secondCalls int
	first := funcEngine{name: "first", gen: func(context.Context, engine.Request) (engine.Result, error) {
		return engine.Result{}, errors.New("not logged in")
	}}
	second := funcEngine{name: "second", gen: func(context.Context, engine.Request) (engine.Result, error) {
		secondCalls++
		return engine.Result{Text: "written"}, nil
	}}

	events := make(chan Event, 128)
	r := NewRunner(config.Config{}, []engine.Engine{first, second}, events)
	r.done = context.Background().Done()

	for i := 0; i < 3; i++ {
		res, err := r.generate(context.Background(), engine.Request{Kind: engine.KindWrite})
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if res.Text != "written" {
			t.Errorf("call %d: text = %q", i, res.Text)
		}
	}
	if secondCalls != 3 {
		t.Errorf("survivor was called %d times, want 3", secondCalls)
	}
	if r.engineName != "second" {
		t.Errorf("engineName = %q, want the survivor", r.engineName)
	}

	close(events)
	var warned bool
	for e := range events {
		if w, ok := e.(WarnEvent); ok && strings.Contains(string(w), "first failed") {
			warned = true
		}
	}
	if !warned {
		t.Error("a backend failure should be reported as a warning, not a plain log line")
	}
}

// An exhausted chain returns the last real error, not a generic one.
func TestGenerateReportsTheLastFailure(t *testing.T) {
	only := funcEngine{name: "only", gen: func(context.Context, engine.Request) (engine.Result, error) {
		return engine.Result{}, errors.New("ollama unreachable")
	}}
	r := NewRunner(config.Config{}, []engine.Engine{only}, nil)
	r.done = context.Background().Done()

	_, err := r.generate(context.Background(), engine.Request{})
	if err == nil || !strings.Contains(err.Error(), "ollama unreachable") {
		t.Errorf("err = %v, want the backend's own message", err)
	}
}

// An empty chain is a configuration problem, not a crash.
func TestGenerateWithNoEnginesAtAll(t *testing.T) {
	r := NewRunner(config.Config{}, nil, nil)
	r.done = context.Background().Done()

	_, err := r.generate(context.Background(), engine.Request{})
	if err == nil || !strings.Contains(err.Error(), "no engine available") {
		t.Errorf("err = %v, want a no-engine error", err)
	}
}

func TestRunWithNoEnginesEmitsAnError(t *testing.T) {
	events := make(chan Event, 8)
	NewRunner(config.Config{}, nil, events).Run(context.Background(), Job{})
	close(events)

	var got string
	for e := range events {
		if ev, ok := e.(ErrEvent); ok {
			got = string(ev)
		}
	}
	if !strings.Contains(got, "no generation engine available") {
		t.Errorf("ErrEvent = %q", got)
	}
}

func TestModTimeOfAMissingFileIsZero(t *testing.T) {
	if got := modTime(filepath.Join(t.TempDir(), "nope.md")); got != 0 {
		t.Errorf("modTime = %d, want 0", got)
	}
}
