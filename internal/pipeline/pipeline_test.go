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
	writeCalls int
	// writer returns the article text and whether it stopped on a length limit,
	// keyed by how many write calls have happened.
	writer func(call int) (string, bool)
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
	return config.Config{
		WriteRetries: 0,
		MaxContinue:  3,
		HomeDir:      dir,
		DraftsDir:    dir,
	}
}

func writeSource(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func drain(t *testing.T, cfg config.Config, primary, fallback engine.Engine, job Job) (DoneEvent, string) {
	t.Helper()
	events := make(chan Event, 256)
	done := make(chan struct{})
	var result DoneEvent
	var errText string
	go func() {
		for e := range events {
			switch ev := e.(type) {
			case DoneEvent:
				result = ev
			case ErrEvent:
				errText = string(ev)
			}
		}
		close(done)
	}()
	NewRunner(cfg, primary, fallback, events).Run(context.Background(), job)
	close(events)
	<-done
	return result, errText
}

func TestRunHappyPath(t *testing.T) {
	cfg := testConfig(t)
	eng := &fakeEngine{name: "fake", writer: func(int) (string, bool) { return validArticle("."), false }}
	done, errText := drain(t, cfg, eng, nil, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("unexpected error: %s", errText)
	}
	if done.OutputPath == "" {
		t.Fatal("expected an output path")
	}
	if done.Engine != "fake" {
		t.Errorf("engine = %q, want fake", done.Engine)
	}
	if _, err := os.Stat(done.OutputPath); err != nil {
		t.Errorf("output file missing: %v", err)
	}
}

func TestRunContinuesPastTruncation(t *testing.T) {
	cfg := testConfig(t)
	// First write stops mid-sentence on a length limit; the continuation
	// finishes it. The pipeline must stitch them and pass validation.
	eng := &fakeEngine{name: "fake", writer: func(call int) (string, bool) {
		if call == 1 {
			return validArticle(" and the story is not yet"), true
		}
		return "done, ending cleanly.", false
	}}
	done, errText := drain(t, cfg, eng, nil, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("unexpected error: %s", errText)
	}
	if eng.writeCalls < 2 {
		t.Errorf("expected a continuation call, got %d write calls", eng.writeCalls)
	}
	data, err := os.ReadFile(done.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "ending cleanly.") {
		t.Errorf("continuation not stitched into output")
	}
}

func TestRunFallsBackOnPrimaryError(t *testing.T) {
	cfg := testConfig(t)
	primary := &fakeEngine{name: "claude", failAll: errors.New("network down")}
	fallback := &fakeEngine{name: "ollama", writer: func(int) (string, bool) { return validArticle("."), false }}
	done, errText := drain(t, cfg, primary, fallback, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("unexpected error: %s", errText)
	}
	if done.Engine != "ollama" {
		t.Errorf("engine = %q, want ollama (fallback)", done.Engine)
	}
}
