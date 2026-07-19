// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/sebastienrousseau/draft/internal/claims"
	"github.com/sebastienrousseau/draft/internal/engine"
	"github.com/sebastienrousseau/draft/internal/prompt"
	"github.com/sebastienrousseau/draft/internal/validate"
)

// surgicalEdit is one exact find/replace change to an existing draft.
type surgicalEdit struct {
	Find    string `json:"find"`
	Replace string `json:"replace"`
	Reason  string `json:"reason"`
}

// review enhances an existing draft (job.ReviewPath) with surgical edits
// grounded in the verified claims mined from the job's sources. It never
// rewrites the draft: the model returns a JSON array of exact find/replace
// edits, which are validated for uniqueness and non-overlap before being
// applied from bottom to top. The result must still pass the house rules.
func (r *Runner) review(ctx context.Context, job Job) error {
	r.phase(PhaseResolve, "running")
	draftBytes, err := os.ReadFile(job.ReviewPath)
	if err != nil {
		r.phase(PhaseResolve, "failed")
		return fmt.Errorf("could not read draft to enhance: %w", err)
	}
	r.log("enhancing " + shortPath(r.cfg, job.ReviewPath))
	r.emit(EngineEvent(r.engineName))
	r.phase(PhaseResolve, "done")

	r.phase(PhaseExtract, "running")
	sections, err := r.sections(ctx, job.Sources)
	if err != nil || len(sections) == 0 {
		r.phase(PhaseExtract, "failed")
		return fmt.Errorf("no readable source text to ground the review")
	}
	r.phase(PhaseExtract, "done")

	outputDir := r.datedDir()
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	r.phase(PhaseClaims, "running")
	records, dropped, err := r.extractClaims(ctx, sections, outputDir)
	if err != nil {
		r.phase(PhaseClaims, "failed")
		return err
	}
	r.log(fmt.Sprintf("verified %d claim(s), dropped %d", len(records), dropped))
	ledger := claims.RenderPromptLedger(records, maxPromptClaims, maxPromptClaimChars)
	r.phase(PhaseClaims, "done")

	var research strings.Builder
	for _, s := range sections {
		research.WriteString(s.Body + "\n\n")
	}

	r.phase(PhaseWrite, "running")
	res, err := r.generate(ctx, engine.Request{
		Kind:        engine.KindEdit,
		Prompt:      prompt.Review(research.String(), string(draftBytes), ledger),
		Temperature: editTemperature,
		OnChunk:     func(s string) { r.emit(TokenEvent(s)) },
	})
	if err != nil {
		r.phase(PhaseWrite, "failed")
		return fmt.Errorf("review generation failed: %w", err)
	}
	r.phase(PhaseWrite, "done")

	r.phase(PhaseSave, "running")
	edits, err := parseSurgicalEdits(cleanOutput(res.Text))
	if err != nil {
		r.phase(PhaseSave, "failed")
		return r.saveFailure(outputDir, res.Text, fmt.Errorf("model did not return valid surgical edits: %w", err))
	}
	enhanced, err := applySurgicalEdits(string(draftBytes), edits)
	if err != nil {
		r.phase(PhaseSave, "failed")
		return r.saveFailure(outputDir, res.Text, fmt.Errorf("surgical edits failed to apply: %w", err))
	}
	if errs := validate.Errors(enhanced); len(errs) > 0 {
		r.phase(PhaseSave, "failed")
		return r.saveFailure(outputDir, enhanced, fmt.Errorf("enhanced draft broke the rules:\n- %s", strings.Join(errs, "\n- ")))
	}
	if err := os.WriteFile(job.ReviewPath, []byte(strings.TrimRight(enhanced, "\n")+"\n"), 0o644); err != nil {
		r.phase(PhaseSave, "failed")
		return err
	}
	r.cleanupArtifacts()
	r.log(fmt.Sprintf("applied %d surgical edit(s)", len(edits)))
	r.phase(PhaseSave, "done")
	r.emit(DoneEvent{OutputPath: job.ReviewPath, Words: validate.WordCount(enhanced), Mode: "review", Engine: r.engineName})
	return nil
}

// parseSurgicalEdits extracts the JSON array of edits from a model response,
// tolerating any chain-of-thought preamble before it.
func parseSurgicalEdits(s string) ([]surgicalEdit, error) {
	if idx := strings.LastIndex(s, "</think>"); idx >= 0 {
		s = s[idx+len("</think>"):]
	}
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end < start {
		return nil, fmt.Errorf("no JSON array found")
	}
	var edits []surgicalEdit
	if err := json.Unmarshal([]byte(s[start:end+1]), &edits); err != nil {
		return nil, err
	}
	return edits, nil
}

// applySurgicalEdits applies validated edits to source. Each find must appear
// exactly once and carry an allowed reason; edits must not overlap. They are
// applied from bottom to top so earlier offsets stay valid.
func applySurgicalEdits(source string, edits []surgicalEdit) (string, error) {
	allowed := map[string]bool{
		"banned word": true, "generic": true, "repeated opening": true,
		"forced choppiness": true, "weak ending": true, "filler": true,
		"factual correction": true,
	}
	type span struct {
		start, end int
		replace    string
	}
	spans := make([]span, 0, len(edits))
	for _, e := range edits {
		if !allowed[e.Reason] {
			return "", fmt.Errorf("unsupported reason %q", e.Reason)
		}
		if e.Find == "" {
			return "", fmt.Errorf("empty find text")
		}
		if c := strings.Count(source, e.Find); c != 1 {
			return "", fmt.Errorf("find text occurs %d times, expected 1: %.80q", c, e.Find)
		}
		st := strings.Index(source, e.Find)
		spans = append(spans, span{start: st, end: st + len(e.Find), replace: e.Replace})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	for i := 1; i < len(spans); i++ {
		if spans[i].start < spans[i-1].end {
			return "", fmt.Errorf("overlapping edits")
		}
	}
	out := source
	for i := len(spans) - 1; i >= 0; i-- {
		out = out[:spans[i].start] + spans[i].replace + out[spans[i].end:]
	}
	return out, nil
}
