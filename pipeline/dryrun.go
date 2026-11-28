// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"errors"

	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/pdf"
)

// DryRunReport is what a run would do, without doing it.
type DryRunReport struct {
	Sources      []string
	SectionCount int
	// Reader is the document reader the sections came from.
	Reader string
	// Engines maps each request kind to the backend that would serve it.
	Engines map[engine.Kind]string
	// EstCalls is the number of model calls a clean run would make: one per
	// section to extract, plus one to write. Retries and continuations are
	// extra and are reported separately by the caller.
	EstCalls    int
	OutputDir   string
	LedgerFound bool
}

// DryRun reports what a run would do and returns without calling a model.
//
// It runs the real resolve and sectioning path — the deterministic ~110 ms of a
// run — so a dry run that succeeds is evidence the sources are readable, not
// just a guess. Committing to a ten-minute run should not be the only way to
// find out that a PDF is a scan.
func (r *Runner) DryRun(ctx context.Context, job Job) (DryRunReport, error) {
	reader := r.cfg.Reader
	if reader == "" {
		reader = pdf.ReaderPDFToText
	}
	rep := DryRunReport{
		Reader:    reader,
		Sources:   job.Sources,
		OutputDir: r.datedDir(),
		Engines: map[engine.Kind]string{
			engine.KindExtract: r.EngineFor(engine.KindExtract),
			engine.KindWrite:   r.EngineFor(engine.KindWrite),
			engine.KindEdit:    r.EngineFor(engine.KindEdit),
		},
	}
	if len(job.Sources) == 0 {
		return rep, errors.New("no source files")
	}

	sections, err := r.sections(ctx, job.Sources)
	if err != nil {
		return rep, err
	}
	if len(sections) == 0 {
		return rep, errors.New("no readable text extracted from the source(s)")
	}
	rep.SectionCount = len(sections)
	rep.EstCalls = len(sections) + 1

	if _, found := resumableLedger(rep.OutputDir, job); found {
		rep.LedgerFound = true
		// A resumable ledger means extraction is already paid for.
		rep.EstCalls = 1
	}
	return rep, nil
}
