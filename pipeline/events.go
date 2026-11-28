// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"time"

	"github.com/sebastienrousseau/draft/engine"
)

// Event types reported during a run. Callers type-switch on them.
type (
	// PhaseEvent updates a pipeline phase's status ("running", "done", "failed").
	PhaseEvent struct {
		Index  int
		Status string
	}
	// LogEvent is a human-readable progress line.
	LogEvent string
	// WarnEvent is a human-readable line reporting something that went wrong
	// but did not stop the run: a backend that failed and was fallen back from,
	// an artefact that could not be saved, an advisory faithfulness warning.
	//
	// It is a distinct type rather than a level field on LogEvent so that
	// existing consumers keep compiling, and so a script can filter on the
	// type it already type-switches over.
	WarnEvent string
	// TokenEvent is a chunk of the article as it streams in.
	TokenEvent string
	// EngineEvent reports which backend is now doing the work.
	EngineEvent string
	// PhaseTiming is how long one phase took and how it ended.
	PhaseTiming struct {
		Index    int
		Name     string
		Status   string // "done" or "failed"
		Duration time.Duration
	}
	// SourceDigest identifies one input by content, so a draft can be traced
	// back to exactly the file that produced it.
	SourceDigest struct {
		Path   string
		SHA256 string
	}
	// DoneEvent is the terminal success event.
	DoneEvent struct {
		OutputPath string
		RawPath    string
		Words      int
		Mode       string
		Engine     string
		// Model, PromptVersion, Sources and LedgerSHA256 form the run
		// manifest. A tool that sells authenticity should be able to say what
		// produced a given draft: which backend and model, which version of
		// the extraction instructions, and which bytes went in.
		Model         string
		PromptVersion string
		Sources       []SourceDigest
		LedgerSHA256  string
		// Duration is the wall-clock time for the whole job, and Timings the
		// per-phase breakdown, so --json output can be compared across runs
		// rather than only read.
		Duration time.Duration
		Timings  []PhaseTiming
		// AttributionPath and ManifestPath locate the provenance pair
		// written beside the set; Sentences and Attributed summarise the
		// attribution so a script can spot a thinly grounded article.
		AttributionPath string
		ManifestPath    string
		Sentences       int
		Attributed      int
		// Usage is what the job's model calls cost, as far as the backends
		// report it. Its Reported flag is false when no backend gave any.
		Usage engine.Usage
	}
	// ErrEvent is the terminal failure event.
	ErrEvent string
)

// Usage re-exports engine.Usage so a consumer of the event stream need not
// import the engine package for the one type.
type Usage = engine.Usage

// Event is the sum type carried on the progress channel.
type Event any

// emit delivers an event to the caller's channel.
//
// Structural and terminal events block until they are accepted — dropping a
// DoneEvent would strand the caller waiting for an outcome that never comes —
// but every send races the run's context, so a consumer that stops draining
// (the dashboard quitting, a caller abandoning the channel) can no longer wedge
// this goroutine forever.
//
// TokenEvents are different: they are emitted from inside the engine's read
// loop, one per streamed chunk, and exist only to animate a preview. They are
// dropped when the buffer is full rather than applying backpressure, because a
// renderer that cannot keep up must slow the preview, never the generation.
// The complete text is returned by the engine regardless of what is dropped.
func (r *Runner) emit(e Event) {
	if r.events == nil {
		return
	}
	if _, lossy := e.(TokenEvent); lossy {
		select {
		case r.events <- e:
		case <-r.done:
		default: // preview frame dropped; the article itself is unaffected
		}
		return
	}
	select {
	case r.events <- e:
	case <-r.done:
	}
}

// phase reports a phase transition and records how long the phase took. The
// timings ride out on DoneEvent, which previously carried no notion of cost at
// all — so "which stage was slow?" could only be answered by watching.
func (r *Runner) phase(index int, status string) {
	switch status {
	case "running":
		r.phaseStart = time.Now()
	case "done", "failed":
		if !r.phaseStart.IsZero() {
			r.timings = append(r.timings, PhaseTiming{
				Index:    index,
				Name:     PhaseNames[index],
				Status:   status,
				Duration: time.Since(r.phaseStart),
			})
			r.phaseStart = time.Time{}
		}
	}
	r.emit(PhaseEvent{Index: index, Status: status})
}
