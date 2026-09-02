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
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/frontmatter"
	"github.com/sebastienrousseau/draft/internal/pdf"
	"github.com/sebastienrousseau/draft/prompt"
	"github.com/sebastienrousseau/draft/rules"
	"github.com/sebastienrousseau/draft/validate"
)

// Prompt budgets for the compact writing ledger.
const (
	maxPromptClaims     = 45
	maxPromptClaimChars = 14000
	extractTemperature  = 0.15
	writeTemperature    = 0.6
	editTemperature     = 0.3
	// maxStemAttempts bounds the search for a free output filename.
	maxStemAttempts = 1000
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
	// DoneEvent is the terminal success event.
	DoneEvent struct {
		OutputPath string
		RawPath    string
		Words      int
		Mode       string
		Engine     string
		// Duration is the wall-clock time for the whole job, and Timings the
		// per-phase breakdown, so --json output can be compared across runs
		// rather than only read.
		Duration time.Duration
		Timings  []PhaseTiming
	}
	// ErrEvent is the terminal failure event.
	ErrEvent string
)

var slugRepeat = regexp.MustCompile(`-{2,}`)

// Runner executes jobs against an ordered chain of engines, advancing to the
// next when one fails and sticking with the survivor.
// chainState is one ordered engine chain plus the cursor into it. Each request
// kind owns one, so extraction failing over to Ollama does not drag writing
// down with it — the two are separately configurable and separately sticky.
type chainState struct {
	engines []engine.Engine
	cur     int
}

func (c *chainState) active() engine.Engine {
	if c == nil || c.cur >= len(c.engines) {
		return nil
	}
	return c.engines[c.cur]
}

type Runner struct {
	cfg config.Config
	// chains holds one chainState per request kind. A Runner built by
	// NewRunner points every kind at the SAME chainState, preserving the
	// single-chain behaviour (including a shared cursor) exactly.
	chains map[engine.Kind]*chainState
	events chan<- Event
	// engineName tracks the backend that actually produced the current output.
	engineName string
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
	started    time.Time
	phaseStart time.Time
	timings    []PhaseTiming
}

// finalize applies the standard post-processing to a raw generation: clean it,
// enforce the house vocabulary, and strip any style-calibration guidance the model
// echoed into the body.
func (r *Runner) finalize(raw string) string {
	return stripCalibrationEcho(normalizeDraft(raw), r.styleText)
}

// Event is the sum type carried on the progress channel.
type Event any

// NewRunner constructs a Runner over one ordered engine chain (see
// engine.Chain), used for every request kind.
func NewRunner(cfg config.Config, engines []engine.Engine, events chan<- Event) *Runner {
	shared := &chainState{engines: engines}
	return &Runner{
		cfg: cfg,
		chains: map[engine.Kind]*chainState{
			engine.KindExtract: shared,
			engine.KindWrite:   shared,
			engine.KindEdit:    shared,
		},
		events: events,
	}
}

// NewRoutedRunner constructs a Runner that resolves a separate chain per
// request kind from cfg, so claim extraction can run against a local model
// while the article itself is written by a session provider.
func NewRoutedRunner(cfg config.Config, events chan<- Event) *Runner {
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
	return &Runner{cfg: cfg, chains: chains, events: events}
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
	records, dropped, err := r.resumeOrExtract(ctx, job, sections, outputDir)
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
	minWords, maxWords := writeBudget(len(records))
	r.writeTokens = writeNumPredict(maxWords, r.cfg.PredictLength)
	if r.engineName == "ollama" {
		r.log(fmt.Sprintf("target %d–%d words for %d claim(s) (cap %d tokens)", minWords, maxWords, len(records), r.writeTokens))
	}
	templates := loadTemplates(r.cfg)
	r.styleText = prompt.EffectiveStyle(templates)
	writePrompt := prompt.Writing(templates, ledger, minWords, maxWords)
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
	outputPath, words, err := r.save(outputDir, markdown)
	if err != nil {
		r.phase(PhaseSave, "failed")
		return err
	}
	r.cleanupArtifacts()
	r.log("saved " + shortPath(r.cfg, outputPath))
	r.phase(PhaseSave, "done")
	r.emit(DoneEvent{
		OutputPath: outputPath,
		Words:      words,
		Mode:       "draft",
		Engine:     r.writerName(),
		Duration:   time.Since(r.started),
		Timings:    append([]PhaseTiming(nil), r.timings...),
	})
	return nil
}

// DryRunReport is what a run would do, without doing it.
type DryRunReport struct {
	Sources      []string
	SectionCount int
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
	rep := DryRunReport{
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

// resumeOrExtract reuses a verified ledger from an earlier attempt when
// --resume is set and one is on disk, and otherwise mines the claims afresh.
//
// Extraction is 80-95% of a run's wall clock. When the write phase fails, that
// work was previously thrown away even though the ledger it produced was still
// sitting in the day folder — so a retry re-paid ten minutes for work that had
// already succeeded.
//
// The reused ledger is re-verified against the freshly sectioned sources rather
// than taken on trust. Resume therefore cannot weaken grounding: if a source
// changed underneath, the records it no longer supports are dropped here, and a
// wholly changed source resumes to an empty ledger that fails the write phase
// honestly instead of producing an ungrounded draft.
func (r *Runner) resumeOrExtract(ctx context.Context, job Job, sections []pdf.Section, outputDir string) ([]claims.Record, int, error) {
	if !r.cfg.Resume {
		return r.extractClaims(ctx, job, sections, outputDir)
	}

	ledgerPath := ledgerPathFor(outputDir, job)
	data, err := os.ReadFile(ledgerPath)
	if err != nil {
		// No ledger to resume from is not an error: fall through to a normal
		// run rather than refusing to work.
		r.log("no ledger to resume from; extracting claims")
		return r.extractClaims(ctx, job, sections, outputDir)
	}

	var corpus strings.Builder
	for _, s := range sections {
		corpus.WriteString(s.Body + "\n\n")
	}
	records, dropped := claims.ParseLedger(string(data), corpus.String())
	if len(records) == 0 {
		r.warn("the saved ledger no longer verifies against these sources; extracting afresh")
		return r.extractClaims(ctx, job, sections, outputDir)
	}

	r.ledgerPath = ledgerPath
	r.log(fmt.Sprintf("resumed %d claim(s) from %s", len(records), shortPath(r.cfg, ledgerPath)))
	if dropped > 0 {
		r.warn(fmt.Sprintf("dropped %d resumed claim(s) the sources no longer support", dropped))
	}
	return records, dropped, nil
}

// ledgerPathFor names the verified-claim ledger for one job.
//
// The name is derived from the job's sources, not just the date. A date-only
// name meant a second paper drafted the same day silently overwrote the first
// one's ledger — losing the fact-checking artefact --keep-artifacts exists to
// preserve, and leaving nothing for --resume to key on. The source count is
// included so a --merge job cannot collide with a single-source job that
// happens to start from the same file.
func ledgerPathFor(outputDir string, job Job) string {
	stem := "sources"
	if len(job.Sources) > 0 {
		base := filepath.Base(job.Sources[0])
		stem = slugify(strings.TrimSuffix(base, filepath.Ext(base)))
	}
	if len(job.Sources) > 1 {
		stem = fmt.Sprintf("%s-plus-%d", stem, len(job.Sources)-1)
	}
	return filepath.Join(outputDir, time.Now().Format("2006-01-02")+"-"+stem+"-verified-claim-ledger.md")
}

// cleanupArtifacts removes the scratch claim ledger after a successful draft so
// the dated folder holds only finished articles. The --keep-artifacts flag
// preserves it for fact-checking.
func (r *Runner) cleanupArtifacts() {
	if r.cfg.KeepArtifacts || r.ledgerPath == "" {
		return
	}
	if err := os.Remove(r.ledgerPath); err == nil {
		r.log("cleaned up claim ledger (use --keep-artifacts to keep it)")
	}
}

// sections reads and splits every source file. A source that cannot be read is
// skipped with a note rather than failing the run, but if nothing at all could
// be read the reason is returned — and when every failure was a missing text
// layer, the returned error preserves pdf.ErrNoTextLayer so the caller can
// surface its OCR advice instead of a generic "no text" message.
func (r *Runner) sections(ctx context.Context, sources []string) ([]pdf.Section, error) {
	var all []pdf.Section
	var firstErr error
	scanned := 0
	failed := 0

	for _, src := range sources {
		text, err := pdf.Extract(ctx, src)
		if err != nil {
			// Cancellation is not a skippable per-file problem.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			failed++
			if errors.Is(err, pdf.ErrNoTextLayer) {
				scanned++
			}
			if firstErr == nil {
				firstErr = err
			}
			r.warn(fmt.Sprintf("skipped %s: %v", filepath.Base(src), err))
			continue
		}
		all = append(all, pdf.SplitSections(filepath.Base(src), text)...)
	}

	if len(all) == 0 && failed > 0 {
		if scanned == failed {
			// Every source was a scan: %w keeps errors.Is working for callers
			// and keeps the remediation text in the message.
			return nil, fmt.Errorf("no readable text extracted from the source(s): %w", pdf.ErrNoTextLayer)
		}
		return nil, fmt.Errorf("no readable text extracted from the source(s): %w", firstErr)
	}
	return all, nil
}

// extractClaims mines quote-verified claims from every section. The first
// section runs through the engine chain to settle on a working backend; the
// remaining sections then run concurrently on that backend when it is a session
// provider (independent subprocess per call), or sequentially for Ollama (a
// single local model that should not be hit in parallel). Any section that
// fails a parallel call is retried through the chain, so a mid-run provider drop
// still degrades to Ollama.
func (r *Runner) extractClaims(ctx context.Context, job Job, sections []pdf.Section, outputDir string) ([]claims.Record, int, error) {
	ledgerPath := ledgerPathFor(outputDir, job)
	r.ledgerPath = ledgerPath
	raw := make([]string, len(sections))

	extract := func(body string) (string, error) {
		return r.generateText(ctx, engine.Request{Kind: engine.KindExtract, Prompt: prompt.Claim(body), Temperature: extractTemperature})
	}

	// Section 0 settles the engine via the chain.
	r.log(fmt.Sprintf("claim section 1/%d", len(sections)))
	text0, err := extract(sections[0].Body)
	if err != nil {
		return nil, 0, fmt.Errorf("claim extraction failed: %w", err)
	}
	raw[0] = text0

	// Pin the backend section 0 settled on. Parallel workers call it directly
	// rather than going through the chain, so they cannot race on the cursor.
	conc := r.extractConcurrency()
	pinned := r.chainFor(engine.KindExtract).active()
	if pinned == nil {
		return nil, 0, errors.New("no extraction engine available")
	}
	if conc > 1 && len(sections) > 1 {
		r.log(fmt.Sprintf("extracting %d section(s) with %d workers via %s", len(sections)-1, conc, pinned.Name()))
	}

	var mu sync.Mutex
	var failed []int
	if conc > 1 {
		sem := make(chan struct{}, conc)
		var wg sync.WaitGroup
		for i := 1; i < len(sections); i++ {
			wg.Add(1)
			sem <- struct{}{}
			go func(i int) {
				defer wg.Done()
				defer func() { <-sem }()
				res, err := pinned.Generate(ctx, engine.Request{Kind: engine.KindExtract, Prompt: prompt.Claim(sections[i].Body), Temperature: extractTemperature})
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					failed = append(failed, i)
					return
				}
				raw[i] = res.Text
			}(i)
		}
		wg.Wait()
	} else {
		// Sequential extraction is where a run actually spends its time, so it
		// is where an estimate is worth having: without one there is no way to
		// tell a two-minute run from a twenty-minute one until it ends.
		eta := newETA(len(sections), time.Since(r.started))
		for i := 1; i < len(sections); i++ {
			r.log(fmt.Sprintf("claim section %d/%d%s", i+1, len(sections), eta.remaining(i)))
			started := time.Now()
			text, err := extract(sections[i].Body)
			if err != nil {
				return nil, 0, fmt.Errorf("claim extraction failed: %w", err)
			}
			eta.observe(time.Since(started))
			raw[i] = text
		}
	}

	// Retry any parallel failures through the chain (handles a provider drop).
	sort.Ints(failed)
	for _, i := range failed {
		r.log(fmt.Sprintf("retrying claim section %d/%d", i+1, len(sections)))
		text, err := extract(sections[i].Body)
		if err != nil {
			return nil, 0, fmt.Errorf("claim extraction failed: %w", err)
		}
		raw[i] = text
	}

	var records []claims.Record
	dropped := 0
	for i, sec := range sections {
		secRecords, secDropped := claims.Parse(raw[i], sec.Body)
		records = append(records, secRecords...)
		dropped += secDropped
	}
	if deduped := claims.Dedupe(records); len(deduped) != len(records) {
		r.log(fmt.Sprintf("removed %d duplicate claim(s)", len(records)-len(deduped)))
		records = deduped
	}
	// Report what actually happened. Logging "claims saved" unconditionally
	// after a discarded write tells the user a fact-checking artefact exists
	// when it may not.
	if err := os.WriteFile(ledgerPath, []byte(claims.RenderLedger(records, dropped)+"\n"), 0o644); err != nil {
		r.ledgerPath = ""
		r.warn(fmt.Sprintf("could not save the claim ledger: %v", err))
	} else {
		r.log("claims saved to " + shortPath(r.cfg, ledgerPath))
	}
	return records, dropped, nil
}

// ollamaExtractConcurrency caps how many extraction calls the local backend runs
// at once. A single small GPU is not saturated by one request: with the server
// started at OLLAMA_NUM_PARALLEL>=2, two concurrent extractions measured ~1.8x the
// throughput of one on an 8 GB machine, and a server pinned to a single slot just
// queues the second — so this is safe either way. Two keeps the win without adding
// memory pressure a shared GPU cannot absorb.
const ollamaExtractConcurrency = 2

// extractConcurrency is the number of parallel extraction workers for the settled
// engine: the configured value for a session provider (independent subprocesses),
// and a small, capped amount for Ollama (concurrent requests to one local server).
func (r *Runner) extractConcurrency() int {
	n := r.cfg.ExtractConcurrency
	if n < 1 {
		n = 1
	}
	// Read the extraction chain rather than the last-used engine name: under
	// per-kind routing the writer may be a session provider while extraction
	// runs locally, and it is the local one that must not be over-driven.
	extractor := ""
	if e := r.chainFor(engine.KindExtract).active(); e != nil {
		extractor = e.Name()
	}
	if extractor == "ollama" && n > ollamaExtractConcurrency {
		return ollamaExtractConcurrency
	}
	return n
}

// generateText runs a request through the engine chain and returns its text.
func (r *Runner) generateText(ctx context.Context, req engine.Request) (string, error) {
	res, err := r.generate(ctx, req)
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

// writeBudget scales the target article length to the amount of grounded
// material. Fixed scaffolding (title, lead aside, executive summary, section
// headers) sets a floor; each verified claim then buys a slice of prose. The
// range is clamped to the house minimum and maximum, so a dense source still
// yields a full-length piece while a thin one is not asked to pad.
func writeBudget(claimCount int) (minWords, maxWords int) {
	target := 350 + claimCount*110
	if target > rules.MaxWords {
		target = rules.MaxWords
	}
	maxWords = target
	minWords = target * 3 / 4
	if minWords < rules.MinWords {
		minWords = rules.MinWords
	}
	if maxWords < minWords+150 {
		maxWords = minWords + 150
	}
	return minWords, maxWords
}

// writeNumPredict converts a word budget into an output-token cap (roughly 1.8
// tokens per word plus headroom for markdown and punctuation), never exceeding
// the configured ceiling. Bounding output to the budget is what stops a local
// model from running to its token limit on a thin ledger.
func writeNumPredict(maxWords, ceiling int) int {
	n := maxWords*18/10 + 400
	if ceiling > 0 && n > ceiling {
		n = ceiling
	}
	return n
}

// write runs the initial generation and continues past any length-limit stop.
func (r *Runner) write(ctx context.Context, writePrompt string) (string, error) {
	res, err := r.generate(ctx, engine.Request{
		Kind:        engine.KindWrite,
		Prompt:      writePrompt,
		Temperature: writeTemperature,
		NumPredict:  r.writeTokens,
		OnChunk:     func(s string) { r.emit(TokenEvent(s)) },
	})
	if err != nil {
		return "", fmt.Errorf("generation failed: %w", err)
	}
	text := r.finalize(res.Text)
	if looksTruncated(res, text) {
		text = r.continueGeneration(ctx, text)
	}
	return text, nil
}

// looksTruncated reports whether a generation needs continuing.
//
// Result.Truncated is authoritative when a backend can set it, but only Ollama
// and the stream-json providers can — every other session provider returns
// plain text with no stop reason attached. Relying on the flag alone left the
// continuation machinery dead for most backends, so a length-limited stop
// surfaced far later as a "article appears truncated" rule violation costing a
// whole rewrite. An ending that does not close a sentence is the same signal
// the validator uses, available to every backend, so use it as the fallback.
func looksTruncated(res engine.Result, text string) bool {
	if res.Truncated {
		return true
	}
	trimmed := strings.TrimRight(text, " \t\r\n")
	return trimmed != "" && !validate.EndsSentence(trimmed)
}

// continuePredictTokens bounds each continuation call. A continuation only has
// to finish the current sentence and add a brief conclusion, so it is capped far
// below the main write budget. Giving it the full budget is what made a model
// that ignores length generate another full block and truncate again, looping
// expensively instead of closing out.
const continuePredictTokens = 512

// continueGeneration finishes an article that stopped on a length limit. Each
// continuation is a small, conclusion-focused call; once the continuation budget
// is spent and the model still has not closed on sentence punctuation, the tail
// is trimmed to the last complete sentence. That keeps the draft from being
// rejected as truncated — which would trigger a far more expensive full rewrite —
// while never adding ungrounded text of our own.
func (r *Runner) continueGeneration(ctx context.Context, partial string) string {
	for i := 0; i < r.cfg.MaxContinue; i++ {
		if validate.EndsSentence(strings.TrimRight(partial, " \t\r\n")) {
			return partial
		}
		r.log(fmt.Sprintf("output hit length limit; concluding (%d/%d)", i+1, r.cfg.MaxContinue))
		res, err := r.generate(ctx, engine.Request{
			Kind:        engine.KindWrite,
			Prompt:      prompt.ContinueWriting(partial),
			Temperature: writeTemperature,
			NumPredict:  continuePredictTokens,
			OnChunk:     func(s string) { r.emit(TokenEvent(s)) },
		})
		if err != nil {
			r.warn("continuation failed: " + err.Error())
			break
		}
		cont := r.finalize(res.Text)
		if strings.TrimSpace(cont) == "" {
			break
		}
		partial = strings.TrimRight(partial, " \t\r\n") + " " + strings.TrimLeft(cont, " \t\r\n")
		if !res.Truncated {
			return partial
		}
	}
	if !validate.EndsSentence(strings.TrimRight(partial, " \t\r\n")) {
		if trimmed := trimToLastSentence(partial); trimmed != "" {
			r.log("trimmed a ragged tail to the last complete sentence")
			return trimmed
		}
	}
	return partial
}

// validateWithRetry validates the draft and, on rule violations, re-prompts the
// writer to fix the named problems, up to the configured retry budget.
func (r *Runner) validateWithRetry(ctx context.Context, basePrompt, markdown string, records []claims.Record) (string, error) {
	var errs []string
	for attempt := 0; attempt <= r.cfg.WriteRetries; attempt++ {
		if attempt > 0 {
			r.warn(fmt.Sprintf("write retry %d: %d violation(s)", attempt, len(errs)))
			retryPrompt := basePrompt + "\n\n## FIX THESE PROBLEMS FROM YOUR PREVIOUS DRAFT\nRewrite the whole article so none of these remain. Change only what is needed.\n- " + strings.Join(errs, "\n- ") + "\n"
			res, err := r.generate(ctx, engine.Request{
				Kind:        engine.KindWrite,
				Prompt:      retryPrompt,
				Temperature: writeTemperature,
				NumPredict:  r.writeTokens,
				OnChunk:     func(s string) { r.emit(TokenEvent(s)) },
			})
			if err != nil {
				return markdown, fmt.Errorf("generation failed: %w", err)
			}
			markdown = r.finalize(res.Text)
			if looksTruncated(res, markdown) {
				markdown = r.continueGeneration(ctx, markdown)
			}
		}
		styleErrs := validate.Errors(markdown)
		factErrs, warnings := validate.FaithfulnessWithOptions(markdown, records, r.validateOptions())
		errs = append(append([]string{}, styleErrs...), factErrs...)
		if len(errs) == 0 {
			for _, w := range warnings {
				r.warn("review: " + w)
			}
			return markdown, nil
		}
	}
	return markdown, fmt.Errorf("article failed the rules after %d retr(y/ies):\n- %s", r.cfg.WriteRetries, strings.Join(errs, "\n- "))
}

// generate runs a request against the active engine, advancing along the chain
// on error (a provider that is offline, not logged in, or failing) until one
// succeeds or the chain is exhausted. The advance is sticky: once an engine
// fails the run does not return to it, so a queue of sections is not re-attempted
// against a dead provider.
func (r *Runner) generate(ctx context.Context, req engine.Request) (engine.Result, error) {
	cs := r.chainFor(req.Kind)
	var lastErr error
	for cs != nil && cs.cur < len(cs.engines) {
		// A cancelled run must not keep walking the chain. Without this check
		// Ctrl+C makes every remaining engine fail in turn, each logging a
		// misleading "falling back to ..." on the way out.
		if err := ctx.Err(); err != nil {
			return engine.Result{}, err
		}
		e := cs.engines[cs.cur]
		res, err := e.Generate(ctx, req)
		if err == nil {
			if r.engineName != e.Name() {
				r.engineName = e.Name()
				r.emit(EngineEvent(e.Name()))
			}
			return res, nil
		}
		// Cancellation and timeout are the caller's decision, not a sick
		// backend. Failing over would retry work the user just abandoned.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return engine.Result{}, err
		}
		lastErr = err
		r.warn(fmt.Sprintf("%s failed (%v)", e.Name(), err))
		cs.cur++
		if cs.cur < len(cs.engines) {
			r.engineName = cs.engines[cs.cur].Name()
			r.emit(EngineEvent(r.engineName))
			r.warn("falling back to " + r.engineName)
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no engine available")
	}
	return engine.Result{}, lastErr
}

// writerName is the engine that produced the article, which is the one worth
// reporting when extraction and writing may be different backends.
func (r *Runner) writerName() string {
	if e := r.chainFor(engine.KindWrite).active(); e != nil {
		return e.Name()
	}
	return r.engineName
}

// save writes the article as a day-folder set — source/<stem>-body.md,
// yaml/<stem>-frontmatter.yaml and final/<stem>-final.md under outputDir —
// and returns the final document's path.
func (r *Runner) save(outputDir, markdown string) (string, int, error) {
	_, body := frontmatter.Split(markdown)
	body = strings.TrimSpace(body)

	title := extractTitle(body)
	now := time.Now()
	dateStr := now.Format("2006-01-02")
	base := dateStr + "-" + slugify(title)

	srcDir := filepath.Join(outputDir, "source")
	yamlDir := filepath.Join(outputDir, "yaml")
	finalDir := filepath.Join(outputDir, "final")
	for _, d := range []string{srcDir, yamlDir, finalDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", 0, err
		}
	}

	// Uniquify the trio as a set: a leftover in any one folder bumps all three
	// names together, so the files can never desync.
	//
	// The body file is claimed with O_EXCL rather than a bare existence check,
	// so two runs racing on the same title cannot both pick the same stem
	// between the check and the write. The loop is bounded: an unbounded
	// search would spin forever against an undeletable path.
	var stem, bodyPath, fmPath, finalPath string
	var bodyFile *os.File
	for i := 1; ; i++ {
		if i > maxStemAttempts {
			return "", 0, fmt.Errorf("could not find a free filename for %q after %d attempts", base, maxStemAttempts)
		}
		stem = base
		if i > 1 {
			stem = fmt.Sprintf("%s-%d", base, i)
		}
		bodyPath = filepath.Join(srcDir, stem+"-body.md")
		fmPath = filepath.Join(yamlDir, stem+"-frontmatter.yaml")
		finalPath = filepath.Join(finalDir, stem+"-final.md")
		if fileExists(fmPath) || fileExists(finalPath) {
			continue
		}
		f, err := os.OpenFile(bodyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", 0, err
		}
		bodyFile = f
		break
	}

	if _, err := bodyFile.WriteString(body + "\n"); err != nil {
		_ = bodyFile.Close()
		return "", 0, err
	}
	if err := bodyFile.Close(); err != nil {
		return "", 0, err
	}
	site := frontmatter.SiteFromEnv()
	fmYAML := frontmatter.GenerateWithOptions(body, frontmatter.Options{
		Date: now,
		Slug: strings.TrimPrefix(stem, dateStr+"-"),
		Site: &site,
	})
	if err := os.WriteFile(fmPath, []byte(fmYAML+"\n"), 0o644); err != nil {
		return "", 0, err
	}
	finalMD := frontmatter.Combine(fmYAML, body)
	if err := os.WriteFile(finalPath, []byte(finalMD+"\n"), 0o644); err != nil {
		return "", 0, err
	}

	r.log("saved body: " + shortPath(r.cfg, bodyPath))
	r.log("saved frontmatter: " + shortPath(r.cfg, fmPath))

	return finalPath, validate.WordCount(body), nil
}

// saveFailure preserves the raw output and, if it still looks like an article,
// a needs-review copy, then returns an error describing where they went.
// It reports only the files it actually managed to write: telling the user
// where to find a rescued draft that was never saved sends them looking for a
// path that does not exist.
func (r *Runner) saveFailure(outputDir, markdown string, verr error) error {
	var note strings.Builder
	rawPath := filepath.Join(outputDir, time.Now().Format("2006-01-02")+"-failed-output.txt")
	if err := os.WriteFile(rawPath, []byte(markdown+"\n"), 0o644); err != nil {
		note.WriteString("\nRaw output could not be saved: " + err.Error())
	} else {
		note.WriteString("\nRaw output saved: " + rawPath)
	}
	if validate.LooksLikeArticle(markdown) {
		reviewPath := uniquePath(filepath.Join(outputDir, time.Now().Format("2006-01-02")+"-"+slugify(extractTitle(markdown))+"-needs-review.md"))
		if err := os.WriteFile(reviewPath, []byte(markdown+"\n"), 0o644); err != nil {
			note.WriteString("\nNeeds-review Markdown could not be saved: " + err.Error())
		} else {
			note.WriteString("\nNeeds-review Markdown saved: " + reviewPath)
		}
	}
	return fmt.Errorf("%w%s", verr, note.String())
}

func (r *Runner) datedDir() string {
	return filepath.Join(r.cfg.DraftsDir, time.Now().Format("2006-01-02"))
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
