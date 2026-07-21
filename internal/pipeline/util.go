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

	"github.com/sebastienrousseau/draft/internal/config"
	"github.com/sebastienrousseau/draft/internal/rules"
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
		out = append(out, styleReplacer{regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(w) + `\b`), rules.StyleReplacements[w]})
	}
	return out
}

// enforceStyle swaps every banned cliché word or phrase for its neutral in-style
// equivalent, matching the case of the first character replaced. It repairs the
// most common reason a small local model's otherwise-clean draft fails the house
// rules, avoiding a slow full regeneration that would only introduce fresh
// clichés. It never touches numbers, names, or quotes, so grounding is untouched.
func enforceStyle(md string) string {
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

// normalizeDraft cleans backend noise, drops any leaked reasoning preamble, and
// enforces the house vocabulary — the standard post-processing for generated
// Markdown before it is validated.
func normalizeDraft(s string) string {
	return enforceStyle(stripThinking(cleanOutput(s)))
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

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func shortPath(cfg config.Config, path string) string {
	if cfg.HomeDir != "" && strings.HasPrefix(path, cfg.HomeDir) {
		return "~" + strings.TrimPrefix(path, cfg.HomeDir)
	}
	return path
}
