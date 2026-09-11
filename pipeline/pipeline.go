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
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
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

var slugRepeat = regexp.MustCompile(`-{2,}`)

// chainState is one ordered engine chain plus the cursor into it. Each request
// kind owns one, so extraction failing over to Ollama does not drag writing
// down with it — the two are separately configurable and separately sticky.
type chainState struct {
	engines []engine.Engine
	cur     int
	// demotedAt is when the cursor last advanced past a failing engine.
	demotedAt time.Time
}

// timeNow is a variable so the rehabilitation clock is testable.
var timeNow = time.Now

// rehabilitationDelay is how long a demoted engine stays demoted before the
// chain probes it again.
//
// Stickiness is right for a provider that is genuinely dead — not logged in,
// uninstalled — because retrying it costs a failed call per job. It is wrong
// for one that blipped: a single flaky moment on the first paper otherwise
// demotes a forty-paper queue to the local model for its entire life, and the
// cost of that is every remaining article written by a 4B model instead of
// the backend the user chose. A failed call is cheap and fails fast; a
// silently downgraded queue is expensive and invisible. Probing occasionally
// is the cheaper mistake.
const rehabilitationDelay = 15 * time.Minute

// rehabilitate returns the chain to its preferred engine once a demoted one
// has had long enough to recover, and reports whether it did.
func (c *chainState) rehabilitate(now time.Time) bool {
	if c == nil || c.cur == 0 || c.demotedAt.IsZero() || now.Sub(c.demotedAt) < rehabilitationDelay {
		return false
	}
	c.cur = 0
	c.demotedAt = time.Time{}
	return true
}

func (c *chainState) active() engine.Engine {
	if c == nil || c.cur >= len(c.engines) {
		return nil
	}
	return c.engines[c.cur]
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
	// as the backends report it. Guarded because parallel extraction workers
	// add to it concurrently.
	usageMu sync.Mutex
	usage   engine.Usage
	// ledgerPath is the verified-claim-ledger scratch file for the current run,
	// removed on success unless the user asked to keep artifacts.
	ledgerPath string
	// writeTokens caps output tokens for the writing calls of the current job,
	// sized to the article's word budget (see writeBudget) so a thin ledger does
	// not drive a local model to pad toward its token ceiling.
	writeTokens int
	// styleText is the style-calibration block embedded in the writing prompt, kept
	// so any of it the model echoes verbatim into the draft can be stripped back out.
	styleText string
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
	// cache reuses claim extractions across runs, addressed by the content
	// that produced them. Nil when caching is disabled; every method on it
	// tolerates that, so there is no branch at the call sites.
	cache *extractcache.Cache
	// cacheHits counts reused extractions for this job. Written from the
	// parallel extraction workers, so it is atomic.
	cacheHits atomic.Int64
}

// Event is the sum type carried on the progress channel.
type Event any

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
	// discovered while extracting is not re-discovered when writing.
	byName := map[string]*chainState{}
	for _, k := range []engine.Kind{engine.KindExtract, engine.KindWrite, engine.KindEdit} {
		name := engine.NameFor(cfg, k)
		if existing, ok := byName[name]; ok {
			chains[k] = existing
			continue
		}
		cs := &chainState{engines: engine.ChainFor(cfg, k)}
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
	// Bind emit to this run's cancellation before anything can be emitted, so
	// a consumer that stops draining cannot wedge this goroutine.
	r.done = ctx.Done()
	r.started = time.Now()
	r.timings = nil
	r.sourceDigests = nil
	r.ledgerDigest = ""
	r.wroteWith = nil
	r.attributionPath, r.manifestPath, r.sentences, r.attributed = "", "", 0, 0
	r.usage = engine.Usage{}

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
	r.phase(PhaseExtract, "running")
	sections, err := r.sections(ctx, job.Sources)
	if err != nil {
		r.phase(PhaseExtract, "failed")
		return err
	}
	if len(sections) == 0 {
		r.phase(PhaseExtract, "failed")
		return fmt.Errorf("no readable text extracted from the source(s)")
	}
	r.log(fmt.Sprintf("read %d section(s)", len(sections)))
	r.phase(PhaseExtract, "done")

	outputDir := r.datedDir()
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	// Phase 2: extract claims, section by section — unless a ledger from an
	// earlier attempt is already on disk and still verifies.
	r.phase(PhaseClaims, "running")
	records, dropped, err := r.resumeOrExtract(ctx, job, sections, outputDir, r.chainFor(engine.KindExtract))
	if err != nil {
		r.phase(PhaseClaims, "failed")
		return err
	}
	r.log(fmt.Sprintf("verified %d claim(s), dropped %d", len(records), dropped))
	ledger := claims.RenderPromptLedger(records, maxPromptClaims, maxPromptClaimChars)
	r.phase(PhaseClaims, "done")

	// Phase 3: write. Size the article to the grounded material: a handful of
	// claims cannot honestly fill 3000 words, and padding is what both slows
	// local generation and trips the faithfulness checks into a costly retry.
	r.phase(PhaseWrite, "running")
	minWords, maxWords := writeBudget(len(records), r.cfg.Style)
	r.writeTokens = writeNumPredict(maxWords, r.cfg.PredictLength)
	if r.engineName == "ollama" {
		r.log(fmt.Sprintf("target %d–%d words for %d claim(s) (cap %d tokens)", minWords, maxWords, len(records), r.writeTokens))
	}
	templates := loadTemplates(r.cfg)
	r.styleText = prompt.EffectiveStyle(templates)
	style := r.cfg.Style
	style.MinWords, style.MaxWords = minWords, maxWords
	writePrompt := prompt.WritingWithStyle(templates, ledger, style)
	markdown, err := r.write(ctx, writePrompt)
	if err != nil {
		r.phase(PhaseWrite, "failed")
		return err
	}
	r.phase(PhaseWrite, "done")

	// Phase 4: validate, retry, save.
	r.phase(PhaseSave, "running")
	markdown, verr := r.validateWithRetry(ctx, writePrompt, markdown, records)
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

	if _, statErr := os.Stat(ledgerPathFor(rep.OutputDir, job)); statErr == nil {
		rep.LedgerFound = true
		// A resumable ledger means extraction is already paid for.
		rep.EstCalls = 1
	}
	return rep, nil
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

// validateOptions is the validation policy for this run.
func (r *Runner) validateOptions() validate.Options {
	return validate.Options{StrictNumbers: r.cfg.StrictNumbers}
}

func (r *Runner) log(msg string) { r.emit(LogEvent(msg)) }

// warn reports something that went wrong without stopping the run.
func (r *Runner) warn(msg string) { r.emit(WarnEvent(msg)) }
