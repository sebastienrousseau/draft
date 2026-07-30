// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/pipeline"
)

// runHeadless processes the queue without the Bubble Tea UI. Progress logs go to
// stderr and each finished draft's path is printed to stdout, so the command
// composes in scripts and cron jobs. It returns a count of failed jobs.
func runHeadless(ctx context.Context, cfg config.Config, engines []engine.Engine, jobs []pipeline.Job, stdout, stderr io.Writer) int {
	failures := 0
	for i, job := range jobs {
		fmt.Fprintf(stderr, "[%d/%d] %v\n", i+1, len(jobs), job.Sources)
		events := make(chan pipeline.Event, 256)
		runner := pipeline.NewRunner(cfg, engines, events)
		go func() {
			runner.Run(ctx, job)
			close(events)
		}()
		for e := range events {
			switch ev := e.(type) {
			case pipeline.LogEvent:
				fmt.Fprintln(stderr, "  ·", string(ev))
			case pipeline.WarnEvent:
				fmt.Fprintln(stderr, "  !", string(ev))
			case pipeline.EngineEvent:
				fmt.Fprintln(stderr, "  engine:", string(ev))
			case pipeline.DoneEvent:
				fmt.Fprintf(stderr, "  ✓ %d words via %s in %s\n", ev.Words, ev.Engine, ev.Duration.Round(time.Millisecond))
				fmt.Fprintln(stdout, ev.OutputPath)
			case pipeline.ErrEvent:
				fmt.Fprintln(stderr, "  ×", string(ev))
				failures++
			}
		}
	}
	return failures
}

// jobRecord is one line of --json output: a stable, machine-readable summary
// of a single job's outcome.
type jobRecord struct {
	Source string `json:"source"`
	Output string `json:"output,omitempty"`
	Engine string `json:"engine,omitempty"`
	Mode   string `json:"mode,omitempty"`
	Words  int    `json:"words,omitempty"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	// Warnings are the non-fatal problems seen during the run — a backend
	// that was fallen back from, an artefact that could not be saved — so a
	// script can spot a degraded success, not just a failure.
	Warnings []string `json:"warnings,omitempty"`
	// Timings make a run comparable across invocations rather than only
	// readable. Milliseconds because they are the unit a script wants and
	// because a Go duration serialises as an opaque nanosecond count.
	DurationMS int64            `json:"duration_ms,omitempty"`
	PhasesMS   map[string]int64 `json:"phases_ms,omitempty"`
}

// phaseMillis flattens per-phase timings into the JSON shape.
func phaseMillis(timings []pipeline.PhaseTiming) map[string]int64 {
	if len(timings) == 0 {
		return nil
	}
	out := make(map[string]int64, len(timings))
	for _, t := range timings {
		out[t.Name] = t.Duration.Milliseconds()
	}
	return out
}

// runHeadlessJSON processes the queue emitting one JSON object per job to
// stdout (JSON Lines), leaving stderr for human progress. It returns a count
// of failed jobs.
func runHeadlessJSON(ctx context.Context, cfg config.Config, engines []engine.Engine, jobs []pipeline.Job, stdout, stderr io.Writer) int {
	failures := 0
	enc := json.NewEncoder(stdout)
	for i, job := range jobs {
		fmt.Fprintf(stderr, "[%d/%d] %v\n", i+1, len(jobs), job.Sources)
		rec := jobRecord{Source: strings.Join(job.Sources, ",")}

		events := make(chan pipeline.Event, 256)
		runner := pipeline.NewRunner(cfg, engines, events)
		go func() {
			runner.Run(ctx, job)
			close(events)
		}()
		for e := range events {
			switch ev := e.(type) {
			case pipeline.LogEvent:
				fmt.Fprintln(stderr, "  ·", string(ev))
			case pipeline.WarnEvent:
				fmt.Fprintln(stderr, "  !", string(ev))
				rec.Warnings = append(rec.Warnings, string(ev))
			case pipeline.DoneEvent:
				rec.Output, rec.Engine, rec.Mode, rec.Words, rec.OK = ev.OutputPath, ev.Engine, ev.Mode, ev.Words, true
				rec.DurationMS = ev.Duration.Milliseconds()
				rec.PhasesMS = phaseMillis(ev.Timings)
			case pipeline.ErrEvent:
				rec.Error = string(ev)
			}
		}
		if !rec.OK {
			failures++
		}
		if err := enc.Encode(rec); err != nil {
			fmt.Fprintln(stderr, "draft:", err)
		}
	}
	return failures
}
