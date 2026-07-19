// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"

	"github.com/sebastienrousseau/draft/internal/config"
	"github.com/sebastienrousseau/draft/internal/engine"
	"github.com/sebastienrousseau/draft/internal/pipeline"
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
			case pipeline.EngineEvent:
				fmt.Fprintln(stderr, "  engine:", string(ev))
			case pipeline.DoneEvent:
				fmt.Fprintf(stderr, "  ✓ %d words via %s\n", ev.Words, ev.Engine)
				fmt.Fprintln(stdout, ev.OutputPath)
			case pipeline.ErrEvent:
				fmt.Fprintln(stderr, "  ×", string(ev))
				failures++
			}
		}
	}
	return failures
}
