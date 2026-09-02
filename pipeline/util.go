// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/internal/mdspan"
	"github.com/sebastienrousseau/draft/rules"
	"github.com/sebastienrousseau/draft/validate"
)

// sentenceClosers are the runes validate.EndsSentence accepts as a clean end,
// kept in sync so a trimmed tail passes the truncation check.
var sentenceClosers = map[rune]bool{
	'.': true, '!': true, '?': true, '"': true, '\'': true, ')': true, ']': true,
	'”': true, '’': true, '…': true, '»': true,
}

// trimToLastSentence cuts s back to its last complete sentence so a draft that a
// model left mid-thought closes cleanly, rather than being rejected as truncated
// and driven into a costly full rewrite. A closer only counts at a real boundary
// (end of text or followed by whitespace), so a period inside a number such as
// "3.1" is never mistaken for a sentence end. It returns "" when no boundary is
// found, leaving the caller to keep the original text.
func trimToLastSentence(s string) string {
	runes := []rune(s)
	end := -1
	for i := 0; i < len(runes); i++ {
		if !sentenceClosers[runes[i]] {
			continue
		}
		if i == len(runes)-1 || unicode.IsSpace(runes[i+1]) {
			end = i + 1
		}
	}
	if end <= 0 {
		return ""
	}
	return strings.TrimRight(string(runes[:end]), " \t\r\n")
}

var (
	ansiEscape = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	titlePat   = regexp.MustCompile(`(?m)^#\s+(.+)$`)
)

// cleanOutput strips ANSI escapes, carriage returns, and control characters
// that a backend might emit around the Markdown.
func cleanOutput(s string) string {
	s = ansiEscape.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r", "")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 32 {
			return r
		}
		return -1
	}, s)
}

// styleReplacers compiles one case-insensitive matcher per banned term, phrases
// first (longest to shortest) and single words with word boundaries, so
// enforceStyle can rewrite a draft in a single pass without re-parsing the rules.
var styleReplacers = buildStyleReplacers()

type styleReplacer struct {
	re   *regexp.Regexp
	with string
}

func buildStyleReplacers() []styleReplacer {
	phrases := append([]string(nil), rules.BannedPhrases...)
	sort.SliceStable(phrases, func(i, j int) bool { return len(phrases[i]) > len(phrases[j]) })
	var out []styleReplacer
	for _, p := range phrases {
		out = append(out, styleReplacer{regexp.MustCompile(`(?i)` + regexp.QuoteMeta(p)), rules.StyleReplacements[p]})
	}
	for _, w := range rules.BannedWords {
		repl := rules.StyleReplacements[w]
		for _, f := range rules.WordForms(w) {
			// Replace each inflected banned form with the replacement inflected the
			// same way, so "leverages" -> "uses" and "leveraging" -> "using".
			out = append(out, styleReplacer{
				regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(f.Form) + `\b`),
				rules.InflectLike(repl, f.Kind),
			})
		}
	}
	return out
}

// enforceStyle swaps every banned cliché word or phrase for its neutral in-style
// equivalent, matching the case of the first character replaced. It repairs the
// most common reason a small local model's otherwise-clean draft fails the house
// rules, avoiding a slow full regeneration that would only introduce fresh
// clichés.
//
// It never rewrites code or a quoted span. The patterns cannot match a number or
// a name, but they very much can match inside a quotation — and rewriting there
// changes what a source is reported to have said, which is the one edit that
// would void the grounding guarantee. See internal/mdspan.
func enforceStyle(md string) string {
	return mdspan.OutsideProtected(md, applyStyleReplacers)
}

// applyStyleReplacers runs every replacement over one unprotected region.
func applyStyleReplacers(md string) string {
	for _, r := range styleReplacers {
		if r.with == "" {
			continue
		}
		md = r.re.ReplaceAllStringFunc(md, func(m string) string {
			out := []rune(r.with)
			if first := []rune(m)[0]; unicode.IsUpper(first) {
				out[0] = unicode.ToUpper(out[0])
			}
			return string(out)
		})
	}
	return md
}

// leakedThesisLabel matches the skeleton's bold thesis placeholder when a literal
// model copies the label but still writes a real thesis after it
// ("**Opening thesis paragraph.** The real thesis..."). Only the label is removed,
// keeping the real sentence.
var leakedThesisLabel = regexp.MustCompile(`(?i)\*\*\s*opening thesis paragraph\.?\s*\*\*\s*`)

// unfilledThesisLine matches a bold opening-thesis line the model left as a bare
// placeholder — empty or just an ellipsis (`**...**`, `**…**`, `****`). There is
// no real content to keep, so the whole line is dropped; the bold lead thesis is a
// stylistic nicety the validator does not require. A real `**thesis.**` never
// matches, because the inner text is neither empty nor an ellipsis.
var unfilledThesisLine = regexp.MustCompile(`(?m)^\*\*[ \t]*(?:\.{2,}|…)?[ \t]*\*\*[ \t]*\r?\n?`)

// unfilledSectionHeading matches an H2–H6 heading the model left as a bare
// ellipsis placeholder (`## ...`, `### …`). The heading line is dropped — its body
// folds into the surrounding prose — so a copied placeholder neither ships nor
// fails the run for want of a title we cannot invent. The H1 is deliberately not
// dropped (a title is required and every model reliably writes one); a stray
// ellipsis H1 is instead caught by the validator.
var unfilledSectionHeading = regexp.MustCompile(`(?m)^#{2,6}[ \t]*(?:\.{2,}|…)[ \t]*\r?\n?`)

// normalizeDraft cleans backend noise, drops any leaked reasoning preamble and
// unfilled skeleton placeholder, and enforces the house vocabulary — the standard
// post-processing for generated Markdown before it is validated.
func normalizeDraft(s string) string {
	s = leakedThesisLabel.ReplaceAllString(s, "")
	s = unfilledThesisLine.ReplaceAllString(s, "")
	s = unfilledSectionHeading.ReplaceAllString(s, "")
	return enforceStyle(stripThinking(cleanOutput(s)))
}

// repairDuplicates drops paragraphs that nearly duplicate an earlier one,
// returning the repaired article and how many were removed.
//
// A rule violation otherwise costs a full rewrite — the single most expensive
// call in a run, paid up to WriteRetries times — and a near-duplicate
// paragraph is redundant by definition, so removing it needs no model at all.
// enforceStyle established this pattern for banned vocabulary; this is the
// same trade for the other violation a deterministic edit can settle.
//
// It stops before the article would fall under the house minimum: shipping a
// too-short draft is a different violation, not a repair. Over-length is
// deliberately NOT repaired by trimming trailing paragraphs — that cuts the
// conclusion and leaves an article that ends abruptly but passes the
// truncation check, which is worse than asking the model again. Removing
// duplicates often brings an over-long draft back inside the band anyway.
func repairDuplicates(md string) (string, int) {
	dups := validate.DuplicateParagraphIndexes(md)
	if len(dups) == 0 {
		return md, 0
	}
	drop := make(map[int]bool, len(dups))
	for _, i := range dups {
		drop[i] = true
	}

	paras := validate.Paragraphs(md)
	kept := make([]string, 0, len(paras))
	removed := 0
	for i, p := range paras {
		if drop[i] {
			// Re-check the floor against what would remain, so the repair
			// cannot trade one violation for another.
			remaining := validate.WordCount(strings.Join(append(append([]string{}, kept...), paras[i+1:]...), "\n\n"))
			if remaining >= rules.MinWords {
				removed++
				continue
			}
		}
		kept = append(kept, p)
	}
	if removed == 0 {
		return md, 0
	}
	return strings.Join(kept, "\n\n"), removed
}

// collapseSpace normalises a block to single-spaced text for echo comparison.
func collapseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// stripCalibrationEcho removes any prose paragraph the model copied wholesale from
// the style-calibration block (the built-in example, or the user's own templates).
// A small local model sometimes reproduces the tone sample as body text; a
// paragraph whose text is contained verbatim in the calibration guidance is an
// echo, not real content. Comparison ignores line wrapping, and headings are left
// alone so structural calibration still works.
func stripCalibrationEcho(article, styleText string) string {
	cal := collapseSpace(styleText)
	if len(cal) < 40 {
		return article
	}
	blocks := strings.Split(article, "\n\n")
	kept := make([]string, 0, len(blocks))
	for _, b := range blocks {
		nb := collapseSpace(b)
		if len(nb) >= 40 && !strings.HasPrefix(strings.TrimSpace(b), "#") && strings.Contains(cal, nb) {
			continue
		}
		kept = append(kept, b)
	}
	return strings.Join(kept, "\n\n")
}

// stripThinking removes any chain-of-thought preamble and returns the Markdown
// starting at the first H1.
func stripThinking(s string) string {
	if idx := strings.LastIndex(s, "</think>"); idx >= 0 {
		s = s[idx+len("</think>"):]
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "# ") {
			return strings.TrimSpace(strings.Join(lines[i:], "\n"))
		}
	}
	return strings.TrimSpace(s)
}

func extractTitle(s string) string {
	if m := titlePat.FindStringSubmatch(s); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return "draft-article"
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	out = slugRepeat.ReplaceAllString(out, "-")
	if len(out) > 90 {
		out = strings.Trim(out[:90], "-")
	}
	if out == "" {
		return "draft-article"
	}
	return out
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// uniquePath returns path, or the first numbered variant of it that does not
// already exist.
//
// The loop is bounded and treats any Stat error other than "does not exist" as
// good enough to use the candidate. Testing only os.IsNotExist meant a
// directory we cannot stat into — no execute permission, a dead network mount —
// made every candidate look occupied and span the loop forever. Failing to
// write the file afterwards is a reported error; spinning is not.
func uniquePath(path string) string {
	if !fileExists(path) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; i <= maxStemAttempts; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if !fileExists(candidate) {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d%s", base, maxStemAttempts+1, ext)
}

func shortPath(cfg config.Config, path string) string {
	if cfg.HomeDir != "" && strings.HasPrefix(path, cfg.HomeDir) {
		return "~" + strings.TrimPrefix(path, cfg.HomeDir)
	}
	return path
}
