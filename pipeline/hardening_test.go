// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/pdf"
)

// emit used to be an unconditional blocking send, so a consumer that stopped
// draining — the dashboard quitting mid-run — wedged the Runner goroutine
// forever. Structural events must still be delivered, but a cancelled run must
// be able to give up.
func TestEmitAbandonsSendWhenRunIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan Event, 2) // deliberately tiny; nobody drains it

	r := NewRunner(config.Config{}, nil, events)
	r.done = ctx.Done()

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for i := 0; i < 100; i++ {
			r.log("progress line")
		}
	}()

	// It should be blocked now: the buffer is full and nothing is reading.
	select {
	case <-finished:
		t.Fatal("expected emit to block while the run is live and the buffer is full")
	case <-time.After(50 * time.Millisecond):
	}

	cancel()

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("emit did not release after cancellation: the goroutine is leaked")
	}
}

// TokenEvents animate a preview. A renderer that cannot keep up must slow the
// preview, never the generation, so they are dropped rather than blocking.
func TestEmitDropsTokenEventsRatherThanBlocking(t *testing.T) {
	events := make(chan Event, 2)
	r := NewRunner(config.Config{}, nil, events)
	r.done = context.Background().Done() // a live, never-cancelled run

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 10_000; i++ {
			r.emit(TokenEvent("chunk"))
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("TokenEvent emission blocked; the UI must not throttle generation")
	}
}

// A nil channel is a supported configuration (no consumer at all).
func TestEmitToNilChannelIsANoOp(t *testing.T) {
	r := NewRunner(config.Config{}, nil, nil)
	r.log("no consumer")
	r.emit(TokenEvent("no consumer"))
}

// Cancelling a run must not walk the rest of the engine chain: every remaining
// engine would fail in turn, each logging a misleading fallback.
func TestGenerateDoesNotFailOverOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var fallbackTried bool
	primary := &fakeEngine{name: "primary", writer: func(int) (string, bool) { return "", false }}
	fallback := funcEngine{name: "fallback", gen: func(context.Context, engine.Request) (engine.Result, error) {
		fallbackTried = true
		return engine.Result{Text: "should never run"}, nil
	}}

	events := make(chan Event, 64)
	r := NewRunner(config.Config{}, []engine.Engine{primary, fallback}, events)
	r.done = ctx.Done()

	_, err := r.generate(ctx, engine.Request{Kind: engine.KindWrite})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if fallbackTried {
		t.Error("a cancelled run fell over to the next engine")
	}
}

// funcEngine is an engine.Engine backed by a closure.
type funcEngine struct {
	name string
	gen  func(context.Context, engine.Request) (engine.Result, error)
}

func (f funcEngine) Name() string { return f.name }
func (f funcEngine) Generate(ctx context.Context, req engine.Request) (engine.Result, error) {
	return f.gen(ctx, req)
}

// sections now returns why nothing could be read, preserving the sentinel so a
// scanned PDF keeps its OCR advice instead of becoming a generic failure.
func TestSectionsPreservesNoTextLayerSentinel(t *testing.T) {
	// The fixture is a real PDF with no text layer, so this needs the same
	// Poppler the extractor does.
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed")
	}
	src := filepath.Join("..", "internal", "pdf", "testdata", "no-text-layer.pdf")

	r := NewRunner(config.Config{}, nil, nil)
	_, err := r.sections(context.Background(), []string{src})
	if err == nil {
		t.Fatal("expected an error when nothing could be read")
	}
	if !errors.Is(err, pdf.ErrNoTextLayer) {
		t.Errorf("errors.Is(err, ErrNoTextLayer) = false; err = %v", err)
	}
	if !strings.Contains(err.Error(), "OCR") {
		t.Errorf("the remediation advice was lost: %v", err)
	}
}

func TestSectionsSkipsOneBadSourceButKeepsTheRest(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.txt")
	if err := os.WriteFile(good, []byte("Readable source text."), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.xyz")
	if err := os.WriteFile(bad, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRunner(config.Config{}, nil, nil)
	secs, err := r.sections(context.Background(), []string{bad, good})
	if err != nil {
		t.Fatalf("one unreadable source must not fail the run: %v", err)
	}
	if len(secs) == 0 {
		t.Error("the readable source produced no sections")
	}
}

// looksTruncated is what makes continuation work for backends that cannot
// report a stop reason — which is every session provider bar stream-json.
func TestLooksTruncated(t *testing.T) {
	for _, tc := range []struct {
		name string
		res  engine.Result
		text string
		want bool
	}{
		{name: "flag set wins", res: engine.Result{Truncated: true}, text: "A clean ending.", want: true},
		{name: "unterminated tail", text: "The sentence just stops mid", want: true},
		{name: "clean full stop", text: "The sentence ends properly.", want: false},
		{name: "trailing whitespace ignored", text: "Ends properly.  \n\n", want: false},
		{name: "empty is not truncated", text: "", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksTruncated(tc.res, tc.text); got != tc.want {
				t.Errorf("looksTruncated = %v, want %v", got, tc.want)
			}
		})
	}
}

// A successful run must report where the time went.
func TestDoneEventCarriesTimings(t *testing.T) {
	cfg := testConfig(t)
	done, errText, _ := drain(t, cfg, []engine.Engine{okEngine("fake")}, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("run failed: %s", errText)
	}
	if done.Duration <= 0 {
		t.Error("DoneEvent.Duration was not set")
	}
	if len(done.Timings) != NumPhases {
		t.Errorf("got %d phase timings, want %d", len(done.Timings), NumPhases)
	}
	for _, tm := range done.Timings {
		if tm.Name == "" {
			t.Errorf("timing for phase %d has no name", tm.Index)
		}
		if tm.Status != "done" {
			t.Errorf("phase %q status = %q, want done", tm.Name, tm.Status)
		}
	}
}
