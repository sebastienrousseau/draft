// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/frontmatter"
	"github.com/sebastienrousseau/draft/internal/atomicfile"
	"github.com/sebastienrousseau/draft/prompt"
	"github.com/sebastienrousseau/draft/validate"
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
	// The model reviews the article body; frontmatter is set aside and
	// re-attached on save so YAML never reaches the prompt or the rules.
	draftFM, draftBody := frontmatter.Split(string(draftBytes))
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
	records, dropped, err := r.extractClaims(ctx, job, sections, outputDir)
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
		Prompt:      prompt.Review(research.String(), draftBody, ledger),
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
	enhanced, err := applySurgicalEdits(draftBody, edits)
	if err != nil {
		r.phase(PhaseSave, "failed")
		return r.saveFailure(outputDir, res.Text, fmt.Errorf("surgical edits failed to apply: %w", err))
	}
	// Gate on style AND grounding, exactly as the write path does. Checking
	// only the house rules here left --review able to introduce an ungrounded
	// number or an unsupported metric into a draft — and "factual correction"
	// is an allowed edit reason, so that is not a hypothetical path.
	//
	// But judge the EDIT, not the article it was applied to. --review operates
	// on a file the user already has, which may predate a rule, exceed the
	// length band, or read as ungrounded against a ledger mined from whichever
	// sources were supplied today. Failing on a violation the edit did not
	// introduce makes an existing article permanently unreviewable for a reason
	// the review had nothing to do with.
	before := reviewViolations(draftBody, records)
	after := reviewViolations(enhanced, records)
	if introduced := newViolations(before, after); len(introduced) > 0 {
		r.phase(PhaseSave, "failed")
		return r.saveFailure(outputDir, enhanced, fmt.Errorf("enhanced draft broke the rules:\n- %s", strings.Join(introduced, "\n- ")))
	}
	// Pre-existing problems are surfaced rather than enforced, so the user
	// still learns about them.
	for _, e := range before {
		r.warn("pre-existing: " + e)
	}
	_, warnings := validate.Faithfulness(enhanced, records)
	for _, w := range warnings {
		r.warn("review: " + w)
	}

	if err := saveEnhanced(job.ReviewPath, draftFM, enhanced); err != nil {
		r.phase(PhaseSave, "failed")
		return err
	}
	r.cleanupArtifacts()
	r.log(fmt.Sprintf("applied %d surgical edit(s)", len(edits)))
	r.phase(PhaseSave, "done")
	r.emit(DoneEvent{
		OutputPath: job.ReviewPath,
		Words:      validate.WordCount(enhanced),
		Mode:       "review",
		Engine:     r.writerName(),
		Duration:   time.Since(r.started),
		Timings:    append([]PhaseTiming(nil), r.timings...),
	})
	return nil
}

// saveEnhanced writes the enhanced body back to path, re-attaching the
// draft's original frontmatter, and resyncs the generated body/yaml/final
// set when path belongs to one.
func saveEnhanced(path, originalFM, body string) error {
	out := strings.TrimRight(body, "\n") + "\n"
	if originalFM != "" {
		out = frontmatter.Combine(originalFM, body)
	}
	// --review rewrites the user's own article in place. A direct WriteFile
	// truncates it first, so a crash or a full disk between truncate and write
	// destroys the draft with nothing to fall back on. Write a sibling
	// temporary file and rename it over the original instead: rename is atomic
	// within a directory, so the file is either the old draft or the new one.
	if err := atomicfile.Write(path, []byte(out), 0o644); err != nil {
		return err
	}
	if frontmatter.PartOfSet(path) {
		if _, _, _, err := frontmatter.ProcessFile(path, time.Now()); err != nil {
			return fmt.Errorf("draft enhanced but its generated set failed to resync: %w", err)
		}
	}
	return nil
}

// reviewViolations is every hard violation in an article: the house rules plus
// the grounding checks, in one list so two articles can be compared.
func reviewViolations(article string, records []claims.Record) []string {
	errs := validate.Errors(article)
	factErrs, _ := validate.Faithfulness(article, records)
	return append(errs, factErrs...)
}

// Validator messages embed data that an edit can shift without changing what
// is wrong: a word count moves by one, a banned-word list gains an entry. These
// reduce such a message to keys that are stable across an edit.
//
// The coupling to validate's wording is deliberate and narrow — only these two
// shapes carry variable data — and TestViolationMessageShapesAreStable pins it,
// so a change to those messages fails here rather than silently degrading the
// comparison to exact matching.
var (
	bannedListPat = regexp.MustCompile(`^(contains banned (?:words|phrases)): (.+)$`)
	wordCountPat  = regexp.MustCompile(`^article is \d+ words; (minimum|maximum) is \d+$`)
)

func violationKeys(msg string) []string {
	// A list message is one key per item, so an edit that adds a banned word to
	// an article that already had one is still caught.
	if m := bannedListPat.FindStringSubmatch(msg); m != nil {
		items := strings.Split(m[2], ", ")
		keys := make([]string, 0, len(items))
		for _, item := range items {
			keys = append(keys, m[1]+": "+strings.TrimSpace(item))
		}
		return keys
	}
	// A count message is one key per bound: being over length before and after
	// is the same problem even when the number moved.
	if m := wordCountPat.FindStringSubmatch(msg); m != nil {
		return []string{"article word count violates the " + m[1]}
	}
	return []string{msg}
}

// newViolations returns the entries of after that are not already in before —
// the problems this edit is responsible for.
func newViolations(before, after []string) []string {
	existing := map[string]bool{}
	for _, e := range before {
		for _, k := range violationKeys(e) {
			existing[k] = true
		}
	}
	var introduced []string
	for _, e := range after {
		for _, k := range violationKeys(e) {
			if !existing[k] {
				introduced = append(introduced, e)
				break
			}
		}
	}
	return introduced
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
