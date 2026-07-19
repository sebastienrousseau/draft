// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/internal/config"
	"github.com/sebastienrousseau/draft/internal/engine"
)

// fakeEngine is a deterministic stand-in for a real backend, so the pipeline's
// orchestration is exercised without any LLM call.
type fakeEngine struct {
	name       string
	failAll    error
	errOnWrite int // if >0, the Nth write call returns an error
	writeCalls int
	writer     func(call int) (string, bool)
}

func (f *fakeEngine) Name() string { return f.name }

func (f *fakeEngine) Generate(_ context.Context, req engine.Request) (engine.Result, error) {
	if f.failAll != nil {
		return engine.Result{}, f.failAll
	}
	switch req.Kind {
	case engine.KindExtract:
		return engine.Result{Text: extractionResponse}, nil
	default:
		f.writeCalls++
		if f.errOnWrite > 0 && f.writeCalls == f.errOnWrite {
			return engine.Result{}, errors.New("write call failed")
		}
		text, truncated := f.writer(f.writeCalls)
		return engine.Result{Text: text, Truncated: truncated}, nil
	}
}

const source = "The system reached a score of 0.82 on the test set. It used 5x fewer samples than before."

const extractionResponse = `CLAIM: The system reached a score of 0.82 on the test set
SOURCE_QUOTE: "reached a score of 0.82 on the test set"
TYPE: metric
STRENGTH: demonstrated
---`

func validArticle(suffix string) string {
	body := strings.Repeat("The grounded result stands on its own and reads plainly. ", 110)
	return "# The Result That Holds\n\n" +
		"**A single number tells the story.**\n\n" +
		"<!-- lead-start -->\n" +
		`<aside class="post-lead" aria-label="Article summary">` + "\n" +
		`<p class="post-lead-tldr"><strong>TL;DR.</strong> A grounded look.</p>` + "\n" +
		"</aside>\n<!-- lead-end -->\n\n" +
		"> **Executive Summary**\n>\n> - The system reached a score on the test set.\n\n" +
		"## What the result shows\n\n" + body + suffix
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	return config.Config{WriteRetries: 0, MaxContinue: 3, HomeDir: dir, DraftsDir: dir}
}

func writeSource(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func drain(t *testing.T, cfg config.Config, engines []engine.Engine, job Job) (DoneEvent, string, []string) {
	t.Helper()
	events := make(chan Event, 512)
	done := make(chan struct{})
	var result DoneEvent
	var errText string
	var logs []string
	go func() {
		for e := range events {
			switch ev := e.(type) {
			case DoneEvent:
				result = ev
			case ErrEvent:
				errText = string(ev)
			case LogEvent:
				logs = append(logs, string(ev))
			}
		}
		close(done)
	}()
	NewRunner(cfg, engines, events).Run(context.Background(), job)
	close(events)
	<-done
	return result, errText, logs
}

func okEngine(name string) *fakeEngine {
	return &fakeEngine{name: name, writer: func(int) (string, bool) { return validArticle("."), false }}
}

func TestRunHappyPathAndCleanup(t *testing.T) {
	cfg := testConfig(t)
	done, errText, _ := drain(t, cfg, []engine.Engine{okEngine("fake")}, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("unexpected error: %s", errText)
	}
	if done.OutputPath == "" || done.Engine != "fake" {
		t.Fatalf("bad done event: %+v", done)
	}
	if _, err := os.Stat(done.OutputPath); err != nil {
		t.Errorf("output file missing: %v", err)
	}
	// Ledger scratch file must be cleaned up by default.
	if ledgers, _ := filepath.Glob(filepath.Join(filepath.Dir(done.OutputPath), "*-verified-claim-ledger.md")); len(ledgers) != 0 {
		t.Errorf("ledger should have been cleaned up, found: %v", ledgers)
	}
}

func TestRunKeepsArtifacts(t *testing.T) {
	cfg := testConfig(t)
	cfg.KeepArtifacts = true
	done, _, _ := drain(t, cfg, []engine.Engine{okEngine("fake")}, Job{Sources: []string{writeSource(t)}})
	if ledgers, _ := filepath.Glob(filepath.Join(filepath.Dir(done.OutputPath), "*-verified-claim-ledger.md")); len(ledgers) == 0 {
		t.Error("ledger should be kept with --keep-artifacts")
	}
}

func TestRunContinuesPastTruncation(t *testing.T) {
	cfg := testConfig(t)
	eng := &fakeEngine{name: "fake", writer: func(call int) (string, bool) {
		if call == 1 {
			return validArticle(" and the story is not yet"), true
		}
		return "done, ending cleanly.", false
	}}
	done, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("unexpected error: %s", errText)
	}
	if eng.writeCalls < 2 {
		t.Errorf("expected a continuation call, got %d", eng.writeCalls)
	}
	data, _ := os.ReadFile(done.OutputPath)
	if !strings.Contains(string(data), "ending cleanly.") {
		t.Error("continuation not stitched into output")
	}
}

func TestRunFallsBackAlongChain(t *testing.T) {
	cfg := testConfig(t)
	primary := &fakeEngine{name: "claude", failAll: errors.New("offline")}
	secondary := &fakeEngine{name: "codex", failAll: errors.New("not logged in")}
	tertiary := okEngine("ollama")
	done, errText, _ := drain(t, cfg, []engine.Engine{primary, secondary, tertiary}, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("unexpected error: %s", errText)
	}
	if done.Engine != "ollama" {
		t.Errorf("engine = %q, want ollama (end of chain)", done.Engine)
	}
}

func TestRunAllEnginesFail(t *testing.T) {
	cfg := testConfig(t)
	a := &fakeEngine{name: "a", failAll: errors.New("nope")}
	b := &fakeEngine{name: "b", failAll: errors.New("nope")}
	_, errText, _ := drain(t, cfg, []engine.Engine{a, b}, Job{Sources: []string{writeSource(t)}})
	if errText == "" {
		t.Fatal("expected terminal error when every engine fails")
	}
}

func TestRunValidationFailureSavesRawAndReview(t *testing.T) {
	cfg := testConfig(t)
	// Return prose that fails structure validation (no H1/aside/etc).
	eng := &fakeEngine{name: "fake", writer: func(int) (string, bool) {
		return "# Untitled\n\n## S\n\njust some prose that is far too short.", false
	}}
	done, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if errText == "" {
		t.Fatal("expected validation failure error")
	}
	if done.OutputPath != "" {
		t.Error("no article should be saved on validation failure")
	}
	if !strings.Contains(errText, "failed the rules") {
		t.Errorf("error should mention rule failure: %s", errText)
	}
}

func TestRunNoSources(t *testing.T) {
	_, errText, _ := drain(t, testConfig(t), []engine.Engine{okEngine("fake")}, Job{})
	if errText == "" {
		t.Fatal("expected error for empty job")
	}
}

func TestRunNoEngines(t *testing.T) {
	_, errText, _ := drain(t, testConfig(t), nil, Job{Sources: []string{writeSource(t)}})
	if !strings.Contains(errText, "no generation engine") {
		t.Errorf("expected no-engine error, got %q", errText)
	}
}

func TestMergeMultipleSources(t *testing.T) {
	cfg := testConfig(t)
	s1, s2 := writeSource(t), writeSource(t)
	done, errText, _ := drain(t, cfg, []engine.Engine{okEngine("fake")}, Job{Sources: []string{s1, s2}})
	if errText != "" {
		t.Fatalf("unexpected error: %s", errText)
	}
	if done.OutputPath == "" {
		t.Error("expected a merged draft")
	}
}
