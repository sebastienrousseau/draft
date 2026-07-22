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
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sebastienrousseau/draft/internal/claims"
	"github.com/sebastienrousseau/draft/internal/config"
	"github.com/sebastienrousseau/draft/internal/engine"
	"github.com/sebastienrousseau/draft/internal/pdf"
	"github.com/sebastienrousseau/draft/internal/prompt"
	"github.com/sebastienrousseau/draft/internal/rules"
	"github.com/sebastienrousseau/draft/internal/validate"
)

// Prompt budgets for the compact writing ledger.
const (
	maxPromptClaims     = 45
	maxPromptClaimChars = 14000
	extractTemperature  = 0.15
	writeTemperature    = 0.6
	editTemperature     = 0.3
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
	// TokenEvent is a chunk of the article as it streams in.
	TokenEvent string
	// EngineEvent reports which backend is now doing the work.
	EngineEvent string
	// DoneEvent is the terminal success event.
	DoneEvent struct {
		OutputPath string
		RawPath    string
		Words      int
		Mode       string
		Engine     string
	}
	// ErrEvent is the terminal failure event.
	ErrEvent string
)

var slugRepeat = regexp.MustCompile(`-{2,}`)

// Runner executes jobs against an ordered chain of engines, advancing to the
// next when one fails and sticking with the survivor.
type Runner struct {
	cfg     config.Config
	engines []engine.Engine
	cur     int // index of the active engine in the chain
	events  chan<- Event
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
}

// finalize applies the standard post-processing to a raw generation: clean it,
// enforce the house vocabulary, and strip any style-calibration guidance the model
// echoed into the body.
func (r *Runner) finalize(raw string) string {
	return stripCalibrationEcho(normalizeDraft(raw), r.styleText)
}

// Event is the sum type carried on the progress channel.
type Event any

// NewRunner constructs a Runner over an ordered engine chain (see engine.Chain).
func NewRunner(cfg config.Config, engines []engine.Engine, events chan<- Event) *Runner {
	return &Runner{cfg: cfg, engines: engines, events: events}
}

// Run executes one job, reporting progress and a terminal Done/Err event. It
// never closes the events channel; the caller owns its lifecycle.
func (r *Runner) Run(ctx context.Context, job Job) {
	if len(r.engines) == 0 {
		r.emit(ErrEvent("no generation engine available"))
		return
	}
	r.engineName = r.engines[0].Name()
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

	// Phase 2: extract claims, section by section.
	r.phase(PhaseClaims, "running")
	records, dropped, err := r.extractClaims(ctx, sections, outputDir)
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
	r.emit(DoneEvent{OutputPath: outputPath, Words: words, Mode: "draft", Engine: r.engineName})
	return nil
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

// sections reads and splits every source file.
func (r *Runner) sections(ctx context.Context, sources []string) ([]pdf.Section, error) {
	var all []pdf.Section
	for _, src := range sources {
		text, err := pdf.Extract(ctx, src)
		if err != nil {
			r.log(fmt.Sprintf("skipped %s: %v", filepath.Base(src), err))
			continue
		}
		all = append(all, pdf.SplitSections(filepath.Base(src), text)...)
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
func (r *Runner) extractClaims(ctx context.Context, sections []pdf.Section, outputDir string) ([]claims.Record, int, error) {
	ledgerPath := filepath.Join(outputDir, time.Now().Format("2006-01-02")+"-verified-claim-ledger.md")
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

	conc := r.extractConcurrency()
	pinned := r.engines[r.cur]
	if conc > 1 && len(sections) > 1 {
		r.log(fmt.Sprintf("extracting %d section(s) with %d workers via %s", len(sections)-1, conc, r.engineName))
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
		for i := 1; i < len(sections); i++ {
			r.log(fmt.Sprintf("claim section %d/%d", i+1, len(sections)))
			text, err := extract(sections[i].Body)
			if err != nil {
				return nil, 0, fmt.Errorf("claim extraction failed: %w", err)
			}
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
	_ = os.WriteFile(ledgerPath, []byte(claims.RenderLedger(records, dropped)+"\n"), 0o644)
	r.log("claims saved to " + shortPath(r.cfg, ledgerPath))
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
	if r.engineName == "ollama" && n > ollamaExtractConcurrency {
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
	if res.Truncated {
		text = r.continueGeneration(ctx, text)
	}
	return text, nil
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
			r.log("continuation failed: " + err.Error())
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
			r.log(fmt.Sprintf("write retry %d: %d violation(s)", attempt, len(errs)))
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
			if res.Truncated {
				markdown = r.continueGeneration(ctx, markdown)
			}
		}
		styleErrs := validate.Errors(markdown)
		factErrs, warnings := validate.Faithfulness(markdown, records)
		errs = append(append([]string{}, styleErrs...), factErrs...)
		if len(errs) == 0 {
			for _, w := range warnings {
				r.log("review: " + w)
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
	var lastErr error
	for r.cur < len(r.engines) {
		e := r.engines[r.cur]
		res, err := e.Generate(ctx, req)
		if err == nil {
			if r.engineName != e.Name() {
				r.engineName = e.Name()
				r.emit(EngineEvent(e.Name()))
			}
			return res, nil
		}
		lastErr = err
		r.log(fmt.Sprintf("%s failed (%v)", e.Name(), err))
		r.cur++
		if r.cur < len(r.engines) {
			r.engineName = r.engines[r.cur].Name()
			r.emit(EngineEvent(r.engineName))
			r.log("falling back to " + r.engineName)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no engine available")
	}
	return engine.Result{}, lastErr
}

func (r *Runner) save(outputDir, markdown string) (string, int, error) {
	title := extractTitle(markdown)
	path := uniquePath(filepath.Join(outputDir, time.Now().Format("2006-01-02")+"-"+slugify(title)+".md"))
	if err := os.WriteFile(path, []byte(markdown+"\n"), 0o644); err != nil {
		return "", 0, err
	}
	return path, validate.WordCount(markdown), nil
}

// saveFailure preserves the raw output and, if it still looks like an article,
// a needs-review copy, then returns an error describing where they went.
func (r *Runner) saveFailure(outputDir, markdown string, verr error) error {
	rawPath := filepath.Join(outputDir, time.Now().Format("2006-01-02")+"-failed-output.txt")
	_ = os.WriteFile(rawPath, []byte(markdown+"\n"), 0o644)
	note := "\nRaw output saved: " + rawPath
	if validate.LooksLikeArticle(markdown) {
		reviewPath := uniquePath(filepath.Join(outputDir, time.Now().Format("2006-01-02")+"-"+slugify(extractTitle(markdown))+"-needs-review.md"))
		_ = os.WriteFile(reviewPath, []byte(markdown+"\n"), 0o644)
		note += "\nNeeds-review Markdown saved: " + reviewPath
	}
	return fmt.Errorf("%v%s", verr, note)
}

func (r *Runner) datedDir() string {
	return filepath.Join(r.cfg.DraftsDir, time.Now().Format("2006-01-02"))
}

// emit sends an event without blocking the pipeline if the UI is slow.
func (r *Runner) emit(e Event) {
	if r.events == nil {
		return
	}
	r.events <- e
}

func (r *Runner) phase(index int, status string) { r.emit(PhaseEvent{Index: index, Status: status}) }
func (r *Runner) log(msg string)                 { r.emit(LogEvent(msg)) }
