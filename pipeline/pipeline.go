// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package pipeline orchestrates a single drafting job end to end: extract source
// text, mine quote-verified claims, write a grounded article (continuing past
// length limits and retrying on rule violations), validate it, and save it. It
// is UI-agnostic — progress is reported through an Event channel — and engine-
// agnostic, trying the primary backend and failing over to the fallback if the
// primary errors (for example, when the network drops mid-run).
package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/extractcache"
	"github.com/sebastienrousseau/draft/internal/pdf"
	"github.com/sebastienrousseau/draft/prompt"
	"github.com/sebastienrousseau/draft/rules"
	"github.com/sebastienrousseau/draft/validate"
)

// Phase indices for progress reporting, in execution order.
const (
	PhaseResolve = iota
	PhaseExtract
	PhaseClaims
	PhaseWrite
	PhaseSave
	phaseCount
)

// NumPhases is the number of pipeline phases, exported for sizing UI state.
const NumPhases = phaseCount

// PhaseNames labels each phase for the UI.
var PhaseNames = [phaseCount]string{
	"Resolve source", "Read and section", "Extract claims", "Write article", "Validate and save",
}

// Job is one unit of work. Normally it is one or more resolved source paths that
// produce one draft; when ReviewPath is set, it instead enhances that existing
// draft with surgical edits grounded in the sources.
type Job struct {
	Sources    []string // absolute paths
	ReviewPath string   // if set, enhance this existing draft instead of generating
}

// Runner executes jobs against an ordered chain of engines, advancing to the
// next when one fails and sticking with the survivor. It is UI-agnostic:
// progress is reported through the Event channel, so the dashboard, --print
// and --json are three consumers of the same stream.
//
// A Runner is reused across a queue, which is what lets a dead provider be
// tried once for the whole run rather than once per paper.
type Runner struct {
	cfg config.Config
	// chains holds one chainState per request kind. A Runner built by
	// NewRunner points every kind at the SAME chainState, preserving the
	// single-chain behaviour (including a shared cursor) exactly.
	chains map[engine.Kind]*chainState
	events chan<- Event
	// usageMu guards concurrent updates to usage from parallel extraction workers.
	usageMu sync.Mutex
	// cache reuses claim extractions across runs, addressed by the content
	// that produced them. Nil when caching is disabled; every method on it
	// tolerates that, so there is no branch at the call sites.
	cache *extractcache.Cache
	// cacheHits counts reused extractions for this job. Written from the
	// parallel extraction workers, so it is atomic.
	cacheHits atomic.Int64
	runState
}

// runState holds the per-job mutable state of a Runner, reset as a unit at the
// start of each Run call so nothing from an earlier job in a queue can leak
// into the next.
type runState struct {
	// engineName tracks the backend that actually produced the current output.
	engineName string
	// wroteWith is the engine that wrote the current article when it was not
	// the chain's active one: a refusal by the active writer routes that one
	// article to the next engine without moving the cursor, and the article's
	// provenance must name the backend that actually wrote it.
	wroteWith engine.Engine
	// attributionPath, manifestPath, sentences and attributed describe the
	// provenance pair the current job wrote, for its DoneEvent.
	attributionPath, manifestPath string
	sentences, attributed         int
	// usage accumulates what every model call in the current job cost, as far
	// as the backends report it.
	usage engine.Usage
	// ledgerPath is this run's ledger, named by runToken, and resumedFrom the
	// one a --resume read; both are removed on success unless artifacts are kept.
	ledgerPath, resumedFrom, runToken string
	// writeTokens caps output tokens for the writing calls of the current job,
	// sized to the article's word budget (see writeBudget) so a thin ledger does
	// not drive a local model to pad toward its token ceiling.
	writeTokens int
	// styleText is the style-calibration block embedded in the writing prompt, kept
	// so any of it the model echoes verbatim into the draft can be stripped back out.
	styleText string
	// sourceTerms are banned terms the verified sources themselves use, which
	// finalize must not rewrite. Set by adoptSourceTerms.
	sourceTerms map[string]bool
	// done mirrors the running job's ctx.Done so emit can abandon a send whose
	// consumer has gone away. Set at the top of Run; nil outside a run, which
	// is a valid never-ready channel in a select.
	done <-chan struct{}
	// started, phaseStart and timings record how long the run and each phase
	// took, reported on DoneEvent so a run's cost is measurable rather than
	// merely felt.
	// sourceDigests and ledgerDigest record what went into the current job,
	// reported on DoneEvent as a run manifest.
	sourceDigests []SourceDigest
	ledgerDigest  string
	started       time.Time
	phaseStart    time.Time
	timings       []PhaseTiming
}

// NewRunner constructs a Runner over one ordered engine chain (see
// engine.Chain), used for every request kind.
func NewRunner(cfg config.Config, engines []engine.Engine, events chan<- Event) *Runner {
	cfg.Style = cfg.Style.OrDefault()
	shared := &chainState{engines: engines}
	return &Runner{
		cfg: cfg,
		chains: map[engine.Kind]*chainState{
			engine.KindExtract: shared,
			engine.KindWrite:   shared,
			engine.KindEdit:    shared,
		},
		events: events,
		cache:  extractcache.Open(cfg.CacheDir),
	}
}

// NewRoutedRunner constructs a Runner that resolves a separate chain per
// request kind from cfg, so claim extraction can run against a local model
// while the article itself is written by a session provider.
func NewRoutedRunner(cfg config.Config, events chan<- Event) *Runner {
	cfg.Style = cfg.Style.OrDefault()
	chains := make(map[engine.Kind]*chainState, 3)
	// Kinds configured to the same engine share one chainState, so a fallback
	// discovered while extracting is not re-discovered when writing. Distinct
	// chains that still include the same fallback provider share the Engine
	// instance itself, so a long-lived ACP agent is not started twice.
	byName := map[string]*chainState{}
	byEngine := map[string]engine.Engine{}
	for _, k := range []engine.Kind{engine.KindExtract, engine.KindWrite, engine.KindEdit} {
		name := engine.NameFor(cfg, k)
		if existing, ok := byName[name]; ok {
			chains[k] = existing
			continue
		}
		engs := engine.ChainFor(cfg, k)
		for i, e := range engs {
			if shared, ok := byEngine[e.Name()]; ok {
				engs[i] = shared
			} else {
				byEngine[e.Name()] = e
			}
		}
		cs := &chainState{engines: engs}
		byName[name] = cs
		chains[k] = cs
	}
	return &Runner{cfg: cfg, chains: chains, events: events, cache: extractcache.Open(cfg.CacheDir)}
}

// SetEvents points the Runner at the channel for the next Run. A Runner is
// reused across a queue so it keeps the backend it settled on, but each job
// gets its own channel because the caller closes it when that job ends.
func (r *Runner) SetEvents(events chan<- Event) { r.events = events }

// EngineFor reports the backend currently serving a request kind, for display.
func (r *Runner) EngineFor(k engine.Kind) string {
	if e := r.chainFor(k).active(); e != nil {
		return e.Name()
	}
	return ""
}

// chainFor returns the chain serving a request kind.
func (r *Runner) chainFor(k engine.Kind) *chainState {
	if cs, ok := r.chains[k]; ok {
		return cs
	}
	return r.chains[engine.KindWrite]
}

// Run executes one job, reporting progress and a terminal Done/Err event. It
// never closes the events channel; the caller owns its lifecycle.
func (r *Runner) Run(ctx context.Context, job Job) {
	// Reset all per-job state and bind emit to this run's cancellation before
	// anything can be emitted, so a consumer that stops draining cannot wedge
	// this goroutine.
	r.runState = runState{
		done:    ctx.Done(),
		started: time.Now(),
	}

	// The cursor is deliberately NOT reset here. A Runner reused across a queue
	// keeps the backend it settled on, so a dead provider is tried once for the
	// whole queue rather than once per paper.
	if e := r.chainFor(engine.KindWrite).active(); e != nil {
		r.engineName = e.Name()
	} else {
		r.emit(ErrEvent("no generation engine available"))
		return
	}
	if err := r.run(ctx, job); err != nil {
		r.emit(ErrEvent(err.Error()))
	}
}

func (r *Runner) run(ctx context.Context, job Job) error {
	if job.ReviewPath != "" {
		return r.review(ctx, job)
	}
	// Phase 0: resolve.
	r.phase(PhaseResolve, "running")
	if len(job.Sources) == 0 {
		return fmt.Errorf("no source files")
	}
	r.log(fmt.Sprintf("resolved %d source file(s)", len(job.Sources)))
	r.emit(EngineEvent(r.engineName))
	r.phase(PhaseResolve, "done")

	// Phase 1: read and section.
	sections, err := r.readSections(ctx, job.Sources)
	if err != nil {
		return err
	}

	outputDir := r.datedDir()
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	// Phase 2: extract and verify the claims the writer may use.
	records, ledger, err := r.groundClaims(ctx, job, sections, outputDir)
	if err != nil {
		return err
	}

	// Phase 3: write.
	markdown, writePrompt, style, err := r.compose(ctx, records, ledger)
	if err != nil {
		return err
	}

	// Phase 4: validate, retry, save.
	return r.validateAndSave(ctx, outputDir, writePrompt, markdown, records, style)
}

// readSections is phase 1: read the sources and split them into sections.
func (r *Runner) readSections(ctx context.Context, sources []string) ([]pdf.Section, error) {
	r.phase(PhaseExtract, "running")
	sections, err := r.sections(ctx, sources)
	if err != nil {
		r.phase(PhaseExtract, "failed")
		return nil, err
	}
	if len(sections) == 0 {
		r.phase(PhaseExtract, "failed")
		return nil, fmt.Errorf("no readable text extracted from the source(s)")
	}
	r.log(fmt.Sprintf("read %d section(s)", len(sections)))
	r.phase(PhaseExtract, "done")
	return sections, nil
}

// validateAndSave is phase 4: validate the article (retrying the write when it
// breaks a rule), save the set, and report the finished draft.
func (r *Runner) validateAndSave(ctx context.Context, outputDir, writePrompt, markdown string, records []claims.Record, style rules.Style) error {
	r.phase(PhaseSave, "running")
	markdown, verr := r.validateWithRetry(ctx, writePrompt, markdown, records, style)
	if verr != nil {
		r.phase(PhaseSave, "failed")
		return r.saveFailure(outputDir, markdown, verr)
	}
	outputPath, words, err := r.save(ctx, outputDir, markdown, records)
	if err != nil {
		r.phase(PhaseSave, "failed")
		return err
	}
	r.cleanupArtifacts()
	r.log("saved " + shortPath(r.cfg, outputPath))
	r.phase(PhaseSave, "done")
	r.emit(DoneEvent{
		OutputPath:      outputPath,
		Words:           words,
		Mode:            "draft",
		Engine:          r.writerName(),
		Model:           r.writerModel(),
		PromptVersion:   prompt.ClaimVersion(),
		Sources:         append([]SourceDigest(nil), r.sourceDigests...),
		LedgerSHA256:    r.ledgerDigest,
		Duration:        time.Since(r.started),
		Timings:         append([]PhaseTiming(nil), r.timings...),
		AttributionPath: r.attributionPath,
		ManifestPath:    r.manifestPath,
		Sentences:       r.sentences,
		Attributed:      r.attributed,
		Usage:           r.currentUsage(),
	})
	return nil
}

// Close releases whatever the engines hold open. A one-shot provider holds
// nothing; an ACP agent is a running process that would otherwise outlive
// the run. Safe to call on a Runner that never ran.
func (r *Runner) Close() error {
	var first error
	// Kinds routed to the same engine share it, so it must be closed once.
	// Engines are compared as interfaces only among closers, which are
	// pointer-shaped by nature (a process handle); comparing arbitrary
	// engine values could panic on an uncomparable test double.
	var closed []io.Closer
	for _, cs := range r.chains {
		if cs == nil {
			continue
		}
		for _, e := range cs.engines {
			c, ok := e.(io.Closer)
			if !ok {
				continue
			}
			if slices.Contains(closed, c) {
				continue
			}
			closed = append(closed, c)
			if err := c.Close(); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

// validateOptions is the validation policy for this run.
func (r *Runner) validateOptions() validate.Options {
	return validate.Options{StrictNumbers: r.cfg.StrictNumbers}
}

func (r *Runner) log(msg string) { r.emit(LogEvent(msg)) }

// warn reports something that went wrong without stopping the run.
func (r *Runner) warn(msg string) { r.emit(WarnEvent(msg)) }
