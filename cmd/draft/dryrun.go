// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/pipeline"
)

// runDryRun prints what each job would do and returns a count of jobs that
// could not even be planned. It makes no model calls, so it costs the
// deterministic ~110 ms of a run rather than its ten minutes.
func runDryRun(ctx context.Context, cfg config.Config, runner *pipeline.Runner, jobs []pipeline.Job, stdout, stderr io.Writer) int {
	failures := 0
	for i, job := range jobs {
		if len(jobs) > 1 {
			fmt.Fprintf(stdout, "\n[%d/%d]\n", i+1, len(jobs))
		}
		rep, err := runner.DryRun(ctx, job)
		if err != nil {
			fmt.Fprintln(stderr, "draft:", err)
			failures++
			continue
		}
		printPlan(stdout, cfg, rep)
	}
	return failures
}

func printPlan(w io.Writer, cfg config.Config, rep pipeline.DryRunReport) {
	names := make([]string, 0, len(rep.Sources))
	for _, s := range rep.Sources {
		names = append(names, filepath.Base(s))
	}

	row := func(label, value string) { fmt.Fprintf(w, "  %-16s %s\n", label, value) }

	fmt.Fprintln(w, "Plan")
	row("Sources", fmt.Sprintf("%d  (%s)", len(names), strings.Join(names, ", ")))
	row("Sections", fmt.Sprint(rep.SectionCount))
	row("Engines", fmt.Sprintf("extract: %s · write: %s · edit: %s",
		rep.Engines[engine.KindExtract], rep.Engines[engine.KindWrite], rep.Engines[engine.KindEdit]))

	calls := fmt.Sprintf("~%d", rep.EstCalls)
	if rep.LedgerFound {
		calls += "  (extraction resumable; add --resume)"
	} else {
		calls += fmt.Sprintf("  (%d extract + 1 write, plus up to %d retries)", rep.SectionCount, cfg.WriteRetries)
	}
	row("Model calls", calls)
	row("Output", rep.OutputDir)
}
