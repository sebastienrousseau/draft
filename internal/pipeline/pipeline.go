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
	"strings"
	"time"

	"github.com/sebastienrousseau/draft/internal/claims"
	"github.com/sebastienrousseau/draft/internal/config"
	"github.com/sebastienrousseau/draft/internal/engine"
	"github.com/sebastienrousseau/draft/internal/pdf"
	"github.com/sebastienrousseau/draft/internal/prompt"
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

// Job is one unit of work: one or more resolved source paths that produce one
// draft.
type Job struct {
	Sources []string // absolute paths
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

// Runner executes jobs against a primary engine with an optional fallback.
type Runner struct {
	cfg      config.Config
	primary  engine.Engine
	fallback engine.Engine
	events   chan<- Event
	// engineName tracks the backend that actually produced the current output.
	engineName string
}

// Event is the sum type carried on the progress channel.
type Event any

// NewRunner constructs a Runner. fallback may be nil.
func NewRunner(cfg config.Config, primary, fallback engine.Engine, events chan<- Event) *Runner {
	return &Runner{cfg: cfg, primary: primary, fallback: fallback, events: events}
}

// Run executes one job, reporting progress and a terminal Done/Err event. It
// never closes the events channel; the caller owns its lifecycle.
func (r *Runner) Run(ctx context.Context, job Job) {
	r.engineName = r.primary.Name()
	if err := r.run(ctx, job); err != nil {
		r.emit(ErrEvent(err.Error()))
	}
}

func (r *Runner) run(ctx context.Context, job Job) error {
	// Phase 0: resolve.
	r.phase(PhaseResolve, "running")
	if len(job.Sources) == 0 {
		return fmt.Errorf("no source files")
	}
	r.log(fmt.Sprintf("resolved %d source file(s)", len(job.Sources)))
	r.emit(EngineEvent(r.primary.Name()))
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

	// Phase 3: write.
	r.phase(PhaseWrite, "running")
	templates := loadTemplates(r.cfg)
	writePrompt := prompt.Writing(templates, ledger)
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
	r.log("saved " + shortPath(r.cfg, outputPath))
	r.phase(PhaseSave, "done")
	r.emit(DoneEvent{OutputPath: outputPath, Words: words, Mode: "draft", Engine: r.engineName})
	return nil
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

// extractClaims runs the extraction model over each section and writes the full
// ledger incrementally so a long run leaves a usable artefact even if it stops.
func (r *Runner) extractClaims(ctx context.Context, sections []pdf.Section, outputDir string) ([]claims.Record, int, error) {
	var records []claims.Record
	dropped := 0
	ledgerPath := filepath.Join(outputDir, time.Now().Format("2006-01-02")+"-verified-claim-ledger.md")
	for i, sec := range sections {
		r.log(fmt.Sprintf("claim section %d/%d", i+1, len(sections)))
		res, err := r.generate(ctx, engine.Request{
			Kind:        engine.KindExtract,
			Prompt:      prompt.Claim(sec.Body),
			Temperature: extractTemperature,
		})
		if err != nil {
			return nil, 0, fmt.Errorf("claim extraction failed: %w", err)
		}
		secRecords, secDropped := claims.Parse(res.Text, sec.Body)
		records = append(records, secRecords...)
		dropped += secDropped
		_ = os.WriteFile(ledgerPath, []byte(claims.RenderLedger(records, dropped)+"\n"), 0o644)
	}
	if deduped := claims.Dedupe(records); len(deduped) != len(records) {
		r.log(fmt.Sprintf("removed %d duplicate claim(s)", len(records)-len(deduped)))
		records = deduped
	}
	_ = os.WriteFile(ledgerPath, []byte(claims.RenderLedger(records, dropped)+"\n"), 0o644)
	r.log("claims saved to " + shortPath(r.cfg, ledgerPath))
	return records, dropped, nil
}

// write runs the initial generation and continues past any length-limit stop.
func (r *Runner) write(ctx context.Context, writePrompt string) (string, error) {
	res, err := r.generate(ctx, engine.Request{
		Kind:        engine.KindWrite,
		Prompt:      writePrompt,
		Temperature: writeTemperature,
		OnChunk:     func(s string) { r.emit(TokenEvent(s)) },
	})
	if err != nil {
		return "", fmt.Errorf("generation failed: %w", err)
	}
	text := stripThinking(cleanOutput(res.Text))
	if res.Truncated {
		text = r.continueGeneration(ctx, text)
	}
	return text, nil
}

// continueGeneration completes an article that stopped on a length limit,
// appending continuations until it ends cleanly or the budget is exhausted.
func (r *Runner) continueGeneration(ctx context.Context, partial string) string {
	for i := 0; i < r.cfg.MaxContinue; i++ {
		if validate.EndsSentence(strings.TrimRight(partial, " \t\r\n")) {
			break
		}
		r.log(fmt.Sprintf("output hit length limit; continuing (%d/%d)", i+1, r.cfg.MaxContinue))
		res, err := r.generate(ctx, engine.Request{
			Kind:        engine.KindWrite,
			Prompt:      prompt.ContinueWriting(partial),
			Temperature: writeTemperature,
			OnChunk:     func(s string) { r.emit(TokenEvent(s)) },
		})
		if err != nil {
			r.log("continuation failed: " + err.Error())
			break
		}
		cont := stripThinking(cleanOutput(res.Text))
		if strings.TrimSpace(cont) == "" {
			break
		}
		partial = strings.TrimRight(partial, " \t\r\n") + " " + strings.TrimLeft(cont, " \t\r\n")
		if !res.Truncated {
			break
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
				OnChunk:     func(s string) { r.emit(TokenEvent(s)) },
			})
			if err != nil {
				return markdown, fmt.Errorf("generation failed: %w", err)
			}
			markdown = stripThinking(cleanOutput(res.Text))
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

// generate runs a request on the primary engine, failing over to the fallback
// on error so a mid-run network drop degrades to Ollama instead of aborting.
func (r *Runner) generate(ctx context.Context, req engine.Request) (engine.Result, error) {
	res, err := r.primary.Generate(ctx, req)
	if err == nil {
		return res, nil
	}
	if r.fallback == nil {
		return res, err
	}
	r.log(fmt.Sprintf("%s failed (%v); falling back to %s", r.primary.Name(), err, r.fallback.Name()))
	r.emit(EngineEvent(r.fallback.Name()))
	r.engineName = r.fallback.Name()
	return r.fallback.Generate(ctx, req)
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
