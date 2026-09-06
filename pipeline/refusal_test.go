// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	writes int
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
		e.mu.Lock()
		e.writes++
		e.mu.Unlock()
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
			// The alternate declines the same section, so nothing can answer
			// it and it must be recorded as empty rather than failing the job.
			alternate := &refusingEngine{name: "codex", marker: tc.marker}
			events := make(chan Event, 4096)
			runner := NewRunner(cfg, []engine.Engine{primary, alternate}, events)

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
			if !hasLog(logs, "claude declined this prompt; trying codex for it") {
				t.Errorf("the alternate should have been offered the section, got %v", logs)
			}
			if hasLog(logs, "falling back") {
				t.Errorf("a refusal must not fall back to the next engine, got %v", logs)
			}
			if cur := runner.chainFor(engine.KindExtract).cur; cur != 0 {
				t.Errorf("chain cursor = %d after a refusal; the engine must be kept", cur)
			}
			if dones[0].Engine != "claude" {
				t.Errorf("article written via %q; the preferred engine must still write", dones[0].Engine)
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
	alternate := &refusingEngine{name: "codex", marker: laterSectionMarker}
	events := make(chan Event, 4096)
	runner := NewRunner(cfg, []engine.Engine{primary, alternate}, events)

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

// A declined article that no engine will write cannot be recovered by
// pretending the model is down: the job fails with the refusal, and the next
// job in the queue still gets the configured engine rather than a silently
// downgraded one.
func TestRefusedWriteFailsTheJobButKeepsTheEngineForTheQueue(t *testing.T) {
	cfg := testConfig(t)
	primary := &refusingEngine{name: "claude", refuseWrite: true}
	alternate := &refusingEngine{name: "codex", refuseWrite: true}
	events := make(chan Event, 4096)
	runner := NewRunner(cfg, []engine.Engine{primary, alternate}, events)

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
	if cur := runner.chainFor(engine.KindWrite).cur; cur != 0 {
		t.Errorf("chain cursor = %d after the queue; the engine must be kept", cur)
	}
}

// When the preferred engine declines a section, the next engine in the chain
// is offered that section alone. The cursor does not move: the following
// section goes back to the preferred engine, and the article is still written
// by it.
func TestRefusedSectionIsAnsweredByTheNextEngine(t *testing.T) {
	for _, conc := range []int{1, 4} {
		t.Run(fmt.Sprintf("concurrency %d", conc), func(t *testing.T) {
			cfg := testConfig(t)
			cfg.ExtractConcurrency = conc
			primary := &refusingEngine{name: "claude", marker: laterSectionMarker}
			alternate := okEngine("ollama")
			events := make(chan Event, 4096)
			runner := NewRunner(cfg, []engine.Engine{primary, alternate}, events)

			dones, errs, logs := collect(t, runner, events, Job{Sources: []string{writeMultiSectionSource(t)}})
			if len(errs) != 0 || len(dones) != 1 {
				t.Fatalf("errs=%v dones=%d", errs, len(dones))
			}
			if !hasLog(logs, "claude declined this prompt; trying ollama for it") {
				t.Errorf("the alternate should have been offered the section, got %v", logs)
			}
			if hasLog(logs, "recorded as having no claims") {
				t.Errorf("an answered section must not be recorded as empty, got %v", logs)
			}
			if alternate.extractCalls != 1 {
				t.Errorf("alternate extracted %d section(s), want exactly the declined one", alternate.extractCalls)
			}
			if alternate.writeCalls != 0 || dones[0].Engine != "claude" {
				t.Errorf("the article must still be written by the preferred engine (alternate wrote %d, via %q)", alternate.writeCalls, dones[0].Engine)
			}
			if cur := runner.chainFor(engine.KindExtract).cur; cur != 0 {
				t.Errorf("chain cursor = %d; offering a section must not demote", cur)
			}
		})
	}
}

// A declined article goes to the next engine too. Its provenance then names
// that engine, and the next job still starts with the preferred one.
func TestRefusedWriteIsWrittenByTheNextEngineAndRecorded(t *testing.T) {
	cfg := testConfig(t)
	cfg.Version = "0.0.34-test"
	cfg.OllamaModel = "local-writer"
	primary := &refusingEngine{name: "claude", refuseWrite: true}
	alternate := okEngine("ollama")
	events := make(chan Event, 4096)
	runner := NewRunner(cfg, []engine.Engine{primary, alternate}, events)

	dones, errs, logs := collect(t, runner, events,
		Job{Sources: []string{writeSource(t)}},
		Job{Sources: []string{writeSource(t)}},
	)
	if len(errs) != 0 || len(dones) != 2 {
		t.Fatalf("errs=%v dones=%d", errs, len(dones))
	}
	for i, d := range dones {
		if d.Engine != "ollama" {
			t.Errorf("job %d written via %q, want the alternate that actually wrote it", i, d.Engine)
		}
	}
	if !hasLog(logs, "claude declined this prompt; trying ollama for it") {
		t.Errorf("expected the hand-off to be reported, got %v", logs)
	}
	if primary.writes != 2 {
		t.Errorf("the preferred engine was asked %d time(s); it must be asked again for the next job", primary.writes)
	}
	if cur := runner.chainFor(engine.KindWrite).cur; cur != 0 {
		t.Errorf("chain cursor = %d; a refusal must not demote", cur)
	}
	// The frontmatter names the engine that wrote the article, not the one
	// that was configured.
	stem := strings.TrimSuffix(filepath.Base(dones[0].OutputPath), "-final.md")
	fm, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(dones[0].OutputPath)), "yaml", stem+"-frontmatter.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`draft_engine: "ollama"`, `draft_version: "0.0.34-test"`, `draft_model: "local-writer"`} {
		if !strings.Contains(string(fm), want) {
			t.Errorf("frontmatter lacks %q:\n%s", want, fm)
		}
	}
}

// closingEngine records Close calls, standing in for an ACP agent process.
type closingEngine struct {
	funcEngine
	closed int
	err    error
}

func (c *closingEngine) Close() error { c.closed++; return c.err }

func TestRunnerCloseReleasesEachEngineOnce(t *testing.T) {
	gen := func(context.Context, engine.Request) (engine.Result, error) { return engine.Result{Text: "x"}, nil }
	shared := &closingEngine{funcEngine: funcEngine{name: "acp", gen: gen}, err: errors.New("kill failed")}
	plain := funcEngine{name: "plain", gen: gen}
	r := NewRunner(testConfig(t), []engine.Engine{shared, plain}, nil)
	if err := r.Close(); err == nil || err.Error() != "kill failed" {
		t.Errorf("Close should report the engine's error, got %v", err)
	}
	// NewRunner shares one chain across three kinds; the engine must still
	// be closed once, not three times.
	if shared.closed != 1 {
		t.Errorf("Close called %d time(s), want 1", shared.closed)
	}
	if err := (&Runner{}).Close(); err != nil {
		t.Errorf("Close on an empty Runner: %v", err)
	}
	// The first error is the one reported; later ones are still closed.
	first := &closingEngine{funcEngine: funcEngine{name: "a", gen: gen}, err: errors.New("first")}
	second := &closingEngine{funcEngine: funcEngine{name: "b", gen: gen}, err: errors.New("second")}
	if err := NewRunner(testConfig(t), []engine.Engine{first, second}, nil).Close(); err == nil || err.Error() != "first" {
		t.Errorf("Close reported %v, want the first error", err)
	}
	if first.closed != 1 || second.closed != 1 {
		t.Errorf("closed counts = %d, %d; every closer must be closed", first.closed, second.closed)
	}
}
