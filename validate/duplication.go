// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"fmt"
	"strings"

	"github.com/sebastienrousseau/draft/rules"
)

// Paragraphs splits an article on blank lines, the unit the duplicate check
// works in. Exported so a repair pass can address the same paragraphs the
// check complains about, rather than re-deriving the split and drifting.
func Paragraphs(article string) []string { return blankLinePat.Split(article, -1) }

// DuplicateParagraphIndexes returns the indexes, within Paragraphs(article), of
// paragraphs that nearly duplicate an earlier one. Headings and short
// paragraphs are never reported: a repeated heading is structure, and short
// runs of shared words are not evidence of duplication.
func DuplicateParagraphIndexes(article string) []int {
	type entry struct {
		index int
		shing map[string]bool
	}
	var entries []entry
	for i, para := range Paragraphs(article) {
		para = strings.TrimSpace(para)
		if para == "" || strings.HasPrefix(strings.TrimLeft(para, " \t"), "#") {
			continue
		}
		words := paragraphWords(para)
		if len(words) < duplicateMinWords {
			continue
		}
		entries = append(entries, entry{index: i, shing: shingles(words, shingleK)})
	}

	var dups []int
	for j := range entries {
		for i := 0; i < j; i++ {
			if jaccard(entries[j].shing, entries[i].shing) >= duplicateThreshold {
				dups = append(dups, entries[j].index)
				break
			}
		}
	}
	return dups
}

func duplicateParagraphs(article string) []string {
	paras := Paragraphs(article)
	var dups []string
	for _, idx := range DuplicateParagraphIndexes(article) {
		dups = append(dups, snippet(strings.Join(strings.Fields(paras[idx]), " "), 70))
	}
	switch {
	case len(dups) == 0:
		return nil
	case len(dups) <= 2:
		out := make([]string, 0, len(dups))
		for _, d := range dups {
			out = append(out, fmt.Sprintf("near-duplicate paragraph (>= %.0f%% overlap with an earlier one): %q...", duplicateThreshold*100, d))
		}
		return out
	default:
		return []string{fmt.Sprintf("%d paragraphs nearly duplicate earlier ones (>= %.0f%% overlap); give each section distinct content", len(dups), duplicateThreshold*100)}
	}
}

func writerSentences(text string) []string {
	plain := mdNoisePat.ReplaceAllString(tagPat.ReplaceAllString(text, "\n"), " ")
	var out []string
	for _, line := range strings.Split(plain, "\n") {
		start := 0
		for i := 0; i < len(line); i++ {
			if c := line[i]; c == '.' || c == '!' || c == '?' {
				if seg := strings.TrimSpace(line[start : i+1]); seg != "" {
					out = append(out, seg)
				}
				start = i + 1
			}
		}
		if seg := strings.TrimSpace(line[start:]); seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

func writerTokens(text string) map[string]bool {
	out := map[string]bool{}
	for _, w := range wordTokenPat.FindAllString(strings.ToLower(text), -1) {
		if rules.WriterStopwords[w] {
			continue
		}
		out[strings.TrimSuffix(w, "s")] = true
	}
	return out
}

func paragraphWords(para string) []string {
	return paraWordPat.FindAllString(strings.ToLower(tagPat.ReplaceAllString(para, " ")), -1)
}

func shingles(words []string, k int) map[string]bool {
	out := map[string]bool{}
	if len(words) < k {
		if len(words) > 0 {
			out[strings.Join(words, "\x00")] = true
		}
		return out
	}
	for i := 0; i <= len(words)-k; i++ {
		out[strings.Join(words[i:i+k], "\x00")] = true
	}
	return out
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	return float64(inter) / float64(union)
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sharedTokens(a, b map[string]bool) int {
	n := 0
	for t := range a {
		if b[t] {
			n++
		}
	}
	return n
}
