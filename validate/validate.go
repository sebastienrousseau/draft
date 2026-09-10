// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package validate enforces the house article rules before a draft is saved:
// required structure, length, banned vocabulary, emoji, truncation, and
// faithfulness to the verified claim ledger.
package validate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sebastienrousseau/draft/internal/mdspan"
	"github.com/sebastienrousseau/draft/rules"
)

var (
	h2Pat          = regexp.MustCompile(`(?m)^##\s+.+$`)
	tagPat         = regexp.MustCompile(`<[^>]+>`)
	mdNoisePat     = regexp.MustCompile("[#>*`_]+")
	wordTokenPat   = regexp.MustCompile(`[a-z][a-z-]{4,}`)
	paraWordPat    = regexp.MustCompile(`[a-z0-9]+`)
	blankLinePat   = regexp.MustCompile(`\n\s*\n`)
	bannedWordRe   = compileWordBoundary(bannedWordForms())
	placeholderPat = regexp.MustCompile(`(?mi)^(#{1,6}[ \t]*(\.{2,}|…)[ \t]*$|\*\*[ \t]*(\.{2,}|…)[ \t]*\*\*|\*\*opening thesis paragraph)`)
)

// Duplicate-detection tuning.
const (
	duplicateThreshold = 0.8
	shingleK           = 4
	duplicateMinWords  = 20
)

// Errors returns the hard rule violations that must block a save. An empty
// slice means the draft is publishable.
// defaultStyle is built once. Errors runs on every draft and every retry, and
// rules.DefaultStyle copies the banned-word and phrase slices; computing it per
// call added two allocations to the hot path for a value that never changes.
var defaultStyle = rules.DefaultStyle()

// Errors reports the house-rule violations in a finished body under the default
// house style. See ErrorsWithStyle for the per-style variant.
func Errors(md string) []string {
	return ErrorsWithStyle(md, defaultStyle)
}

// ErrorsWithStyle checks a body against a specific editorial style: the word
// band and banned vocabulary come from the style, the structural rules do not,
// because an H1, a lead aside, an executive summary and section headings are
// the shape of a grounded article rather than a matter of taste.
//
// The default style takes the fast path — a package-level regex compiled once
// — so the common case pays nothing for the configurability. A custom style
// compiles its banned-word matcher on demand.
func ErrorsWithStyle(md string, style rules.Style) []string {
	var errs []string
	if !strings.HasPrefix(md, rules.H1Prefix) {
		errs = append(errs, "body-only mode must start with a Markdown H1")
	}
	if !strings.Contains(md, rules.PostLeadAsideMarker) {
		errs = append(errs, "missing post-lead aside")
	}
	if !strings.Contains(md, rules.ExecSummaryMarker) {
		errs = append(errs, "missing Executive Summary")
	}
	if !h2Pat.MatchString(md) {
		errs = append(errs, "missing section headings")
	}
	if placeholderPat.MatchString(md) {
		errs = append(errs, "contains an unfilled skeleton placeholder (title, heading, or thesis)")
	}
	// Both bounds, not just the floor. The writing prompt asks for
	// MinWords–MaxWords and the style declares that as the band for a finished
	// draft, but only the minimum was ever checked — so a runaway draft was
	// told one thing and held to another.
	if w := WordCount(md); w < style.MinWords {
		errs = append(errs, fmt.Sprintf("article is %d words; minimum is %d", w, style.MinWords))
	} else if w > style.MaxWords {
		errs = append(errs, fmt.Sprintf("article is %d words; maximum is %d", w, style.MaxWords))
	}
	if ContainsEmoji(md) {
		errs = append(errs, "contains emoji")
	}
	// Scan the writer's own prose only. Code and quoted spans are exempt
	// because enforceStyle refuses to rewrite them — attributing different
	// words to a source is worse than a cliché — and a rule the repair pass
	// cannot satisfy would otherwise drive an unfixable rewrite loop.
	//
	// The spans are located only once something has been found to exonerate.
	// Locating them costs a scan of the whole document, and the overwhelming
	// majority of drafts reach here with no banned vocabulary at all — the
	// repair pass has already removed it — so paying up front made every
	// clean draft subsidise the rare dirty one.
	lowered := strings.ToLower(md)
	wordRe := bannedWordRe
	bannedPhrases := rules.BannedPhrases
	if !isDefaultVocabulary(style) {
		wordRe = compileWordBoundary(style.BannedWordForms())
		bannedPhrases = style.BannedPhrases
	}
	wordHits := wordRe.FindAllStringIndex(lowered, -1)
	var phrases []string
	for _, p := range bannedPhrases {
		if strings.Contains(lowered, p) {
			phrases = append(phrases, p)
		}
	}

	if len(wordHits) > 0 || len(phrases) > 0 {
		// Spans are located in the lowered text so offsets agree: lowercasing
		// cannot move a quote or a backtick, which are all the delimiters
		// involved.
		protected := mdspan.Protected(lowered)
		var words []string
		for _, loc := range wordHits {
			if mdspan.Covers(protected, loc[0]) {
				continue
			}
			words = append(words, lowered[loc[0]:loc[1]])
		}
		if len(words) > 0 {
			errs = append(errs, "contains banned words: "+strings.Join(dedupeStrings(words), ", "))
		}
		kept := phrases[:0]
		for _, p := range phrases {
			if indexOutsideProtected(lowered, p, protected) >= 0 {
				kept = append(kept, p)
			}
		}
		phrases = kept
	}
	if len(phrases) > 0 {
		errs = append(errs, "contains banned phrases: "+strings.Join(phrases, ", "))
	}
	return errs
}

func wordBoundaryContains(s, term string) bool {
	for idx := 0; idx <= len(s)-len(term); {
		j := strings.Index(s[idx:], term)
		if j < 0 {
			return false
		}
		pos := idx + j
		before := pos == 0 || !isWordByte(s[pos-1])
		end := pos + len(term)
		after := end >= len(s) || !isWordByte(s[end])
		if before && after {
			return true
		}
		idx = pos + 1
	}
	return false
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// bannedWordForms expands the banned vocabulary to include common inflections, so
// "leverages" and "leveraging" are caught, not only the base "leverage".
func bannedWordForms() []string {
	var forms []string
	for _, w := range rules.BannedWords {
		for _, f := range rules.WordForms(w) {
			forms = append(forms, f.Form)
		}
	}
	return forms
}

// isDefaultVocabulary reports whether a style's banned lists are the built-in
// ones, so the default path can reuse the package-level compiled regex instead
// of building one per call.
func isDefaultVocabulary(style rules.Style) bool {
	return sameStrings(style.BannedWords, rules.BannedWords) && sameStrings(style.BannedPhrases, rules.BannedPhrases)
}

func compileWordBoundary(words []string) *regexp.Regexp {
	if len(words) == 0 {
		// A pattern that never matches, so a style that bans no words is not
		// an "empty alternation matches everywhere" bug.
		return regexp.MustCompile(`\b\B`)
	}
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = regexp.QuoteMeta(w)
	}
	return regexp.MustCompile(`\b(?:` + strings.Join(quoted, "|") + `)\b`)
}

func containsAny(s string, terms []string) bool {
	for _, t := range terms {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}
