// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/prompt"
	"github.com/sebastienrousseau/draft/rules"
	"github.com/sebastienrousseau/draft/validate"
)

// finalize applies the standard post-processing to a raw generation: clean it,
// enforce the house vocabulary, and strip any style-calibration guidance the model
// echoed into the body.
func (r *Runner) finalize(raw string) string {
	return stripCalibrationEcho(normalizeDraft(raw), r.styleText)
}

// writeBudget scales the target article length to the amount of grounded
// material. Fixed scaffolding (title, lead aside, executive summary, section
// headers) sets a floor; each verified claim then buys a slice of prose. The
// range is clamped to the house minimum and maximum, so a dense source still
// yields a full-length piece while a thin one is not asked to pad.
func writeBudget(claimCount int, style rules.Style) (minWords, maxWords int) {
	target := 350 + claimCount*110
	if target > style.MaxWords {
		target = style.MaxWords
	}
	maxWords = target
	minWords = target * 3 / 4
	if minWords < style.MinWords {
		minWords = style.MinWords
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
		// Repair what a deterministic edit can settle before spending another
		// generation on it. A rewrite is the most expensive call in the run.
		if repaired, removed := repairDuplicates(markdown); removed > 0 {
			if len(validate.ErrorsWithStyle(repaired, r.cfg.Style)) <= len(validate.ErrorsWithStyle(markdown, r.cfg.Style)) {
				markdown = repaired
				r.log(fmt.Sprintf("removed %d near-duplicate paragraph(s) without regenerating", removed))
			}
		}

		styleErrs := validate.ErrorsWithStyle(markdown, r.cfg.Style)
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
