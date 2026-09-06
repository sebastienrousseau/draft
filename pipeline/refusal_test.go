// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/sebastienrousseau/draft/engine"
)

// A refusal is the model declining one prompt. It used to be indistinguishable
// from a crashed provider, so one declined section demoted the chain for the
// rest of the queue: in auto mode every later paper was quietly written by
// the next provider, and with a single pinned engine the queue stranded on
// "no generation engine available". These tests pin the intended behaviour:
// the engine is kept, and a declined section is a section with no claims.

// refusingEngine answers like okEngine except that it declines any extraction
// prompt quoting marker, the way the API declines a section describing an
// attack. failFirst makes the first call on such a prompt fail with an
// ordinary error instead, so the section goes through the parallel retry
// path before it is declined. refuseWrite declines every article.
type refusingEngine struct {
	name        string
	marker      string
	failFirst   bool
	refuseWrite bool

	mu     sync.Mutex
	marked int // calls so far on a prompt quoting marker
}

func refusal(name string) error { return fmt.Errorf("%s: %w", name, engine.ErrRefused) }

func (e *refusingEngine) Name() string { return e.name }

func (e *refusingEngine) Generate(_ context.Context, req engine.Request) (engine.Result, error) {
	switch req.Kind {
	case engine.KindExtract:
		if e.marker != "" && strings.Contains(req.Prompt, e.marker) {
			// Count by marker, not by prompt text: each claim prompt carries
			// a fresh nonce, so the retry never presents the same string.
			e.mu.Lock()
			e.marked++
			n := e.marked
			e.mu.Unlock()
			if e.failFirst && n == 1 {
				return engine.Result{}, errors.New("transient")
			}
			return engine.Result{}, refusal(e.name)
		}
		return engine.Result{Text: extractionResponse}, nil
	case engine.KindEdit:
		return engine.Result{Text: "[]"}, nil
	default:
		if e.refuseWrite {
			return engine.Result{}, refusal(e.name)
		}
		return engine.Result{Text: validArticle(".")}, nil
	}
}

// Section bodies of multiSectionSource, so a test can decline one by name.
const (
	firstSectionMarker = "reached a score of 0.82"
	laterSectionMarker = "Background about the approach"
)

// collect runs each job on one runner and gathers the outcome per job, so a
// test can watch the chain cursor across a queue.
func collect(t *testing.T, runner *Runner, events chan Event, jobs ...Job) (dones []DoneEvent, errs []string, logs []string) {
	t.Helper()
	finished := make(chan struct{})
	go func() {
		for e := range events {
			switch ev := e.(type) {
			case DoneEvent:
				dones = append(dones, ev)
			case ErrEvent:
				errs = append(errs, string(ev))
			case LogEvent:
				logs = append(logs, string(ev))
			case WarnEvent:
				logs = append(logs, string(ev))
			}
		}
		close(finished)
	}()
	for _, job := range jobs {
		runner.Run(context.Background(), job)
	}
	close(events)
	<-finished
	return dones, errs, logs
}

func TestRefusedSectionIsRecordedAsEmptyWithoutDemotingTheChain(t *testing.T) {
	for _, tc := range []struct {
		name   string
		conc   int
		marker string
	}{
		{"parallel, later section", 4, laterSectionMarker},
		{"parallel, first section", 4, firstSectionMarker},
		{"sequential, later section", 1, laterSectionMarker},
		{"sequential, first section", 1, firstSectionMarker},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig(t)
			cfg.ExtractConcurrency = tc.conc
			primary := &refusingEngine{name: "claude", marker: tc.marker}
			fallback := okEngine("ollama")
			events := make(chan Event, 4096)
			runner := NewRunner(cfg, []engine.Engine{primary, fallback}, events)

			dones, errs, logs := collect(t, runner, events, Job{Sources: []string{writeMultiSectionSource(t)}})

			if len(errs) != 0 {
				t.Fatalf("a declined section must not fail the job: %v", errs)
			}
			if len(dones) != 1 || dones[0].OutputPath == "" {
				t.Fatal("expected a saved draft built from the sections that were answered")
			}
			if !hasLog(logs, "recorded as having no claims") {
				t.Errorf("the declined section should be reported, got %v", logs)
			}
			if hasLog(logs, "falling back") {
				t.Errorf("a refusal must not fall back to the next engine, got %v", logs)
			}
			if cur := runner.chainFor(engine.KindExtract).cur; cur != 0 {
				t.Errorf("chain cursor = %d after a refusal; the engine must be kept", cur)
			}
			if fallback.extractCalls != 0 || fallback.writeCalls != 0 {
				t.Errorf("fallback was called (%d extract, %d write); a refusal is not an outage", fallback.extractCalls, fallback.writeCalls)
			}
		})
	}
}

// A section that first fails for an ordinary reason is retried through the
// chain; when that retry is declined the section is still recorded as empty
// rather than failing the job.
func TestRefusedSectionOnTheRetryPathIsRecordedAsEmpty(t *testing.T) {
	cfg := testConfig(t)
	cfg.ExtractConcurrency = 4
	primary := &refusingEngine{name: "claude", marker: laterSectionMarker, failFirst: true}
	events := make(chan Event, 4096)
	runner := NewRunner(cfg, []engine.Engine{primary, okEngine("ollama")}, events)

	dones, errs, logs := collect(t, runner, events, Job{Sources: []string{writeMultiSectionSource(t)}})

	if len(errs) != 0 {
		t.Fatalf("unexpected failure: %v", errs)
	}
	if len(dones) != 1 || dones[0].OutputPath == "" {
		t.Fatal("expected a saved draft")
	}
	if !hasLog(logs, "retrying claim section 2/3") {
		t.Errorf("the transient failure should have been retried, got %v", logs)
	}
	if !hasLog(logs, "claim section 2/3: claude: "+engine.ErrRefused.Error()) {
		t.Errorf("the retry's refusal should be reported for section 2, got %v", logs)
	}
	if cur := runner.chainFor(engine.KindExtract).cur; cur != 0 {
		t.Errorf("chain cursor = %d; the engine must be kept", cur)
	}
}

// A declined article cannot be recovered by pretending the model is down: the
// job fails with the refusal, and the next job in the queue still gets the
// configured engine rather than a silently downgraded one.
func TestRefusedWriteFailsTheJobButKeepsTheEngineForTheQueue(t *testing.T) {
	cfg := testConfig(t)
	primary := &refusingEngine{name: "claude", refuseWrite: true}
	fallback := okEngine("ollama")
	events := make(chan Event, 4096)
	runner := NewRunner(cfg, []engine.Engine{primary, fallback}, events)

	_, errs, logs := collect(t, runner, events,
		Job{Sources: []string{writeSource(t)}},
		Job{Sources: []string{writeSource(t)}},
	)

	if len(errs) != 2 {
		t.Fatalf("both jobs should fail with the refusal, got %v", errs)
	}
	for _, e := range errs {
		if !strings.Contains(e, engine.ErrRefused.Error()) {
			t.Errorf("job error should carry the refusal, got %q", e)
		}
	}
	if hasLog(logs, "falling back") {
		t.Errorf("a declined article must not fall back, got %v", logs)
	}
	if fallback.writeCalls != 0 {
		t.Errorf("fallback wrote %d article(s); a refusal is not an outage", fallback.writeCalls)
	}
	if cur := runner.chainFor(engine.KindWrite).cur; cur != 0 {
		t.Errorf("chain cursor = %d after the queue; the engine must be kept", cur)
	}
}
