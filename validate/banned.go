// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"regexp"
	"sort"
)

// bannedScanner finds banned word forms in lowered text. Most forms are a
// single word-token, so they are matched by tokenising the text once and
// looking each word up in a set — O(text) with an O(1) lookup per word, which
// replaced a `\b(form1|form2|…)\b` regex whose 140-way alternation dominated
// the validation benchmark. The few forms that are not a single token (hyphen-
// ated ones like "cutting-edge") keep a small word-boundary regex so their
// cross-hyphen match semantics are preserved exactly.
type bannedScanner struct {
	set    map[string]struct{}
	hyphen *regexp.Regexp // matches the non-single-token forms, or nil
}

// newBannedScanner splits forms into the single-token set and a small regex for
// the rest. Forms are already lower-case, matching the lowered text they scan.
func newBannedScanner(forms []string) *bannedScanner {
	s := &bannedScanner{set: make(map[string]struct{}, len(forms))}
	var multi []string
	for _, f := range forms {
		if isSingleToken(f) {
			s.set[f] = struct{}{}
		} else if f != "" {
			multi = append(multi, f)
		}
	}
	if len(multi) > 0 {
		s.hyphen = compileWordBoundary(multi)
	}
	return s
}

// isSingleToken reports whether f is one maximal run of word bytes — the case
// where a set lookup is exactly equivalent to a \bf\b regex match.
func isSingleToken(f string) bool {
	if f == "" {
		return false
	}
	for i := 0; i < len(f); i++ {
		if !isWordByte(f[i]) {
			return false
		}
	}
	return true
}

// findAll returns the [start, end) byte spans of every banned word in lowered,
// in text order, so the caller can check each against protected spans and
// report it exactly as the old regex did.
func (b *bannedScanner) findAll(lowered string) [][]int {
	var hits [][]int
	for i := 0; i < len(lowered); {
		if !isWordByte(lowered[i]) {
			i++
			continue
		}
		j := i
		for j < len(lowered) && isWordByte(lowered[j]) {
			j++
		}
		if _, ok := b.set[lowered[i:j]]; ok {
			hits = append(hits, []int{i, j})
		}
		i = j
	}
	if b.hyphen != nil {
		hits = append(hits, b.hyphen.FindAllStringIndex(lowered, -1)...)
		sort.Slice(hits, func(a, c int) bool { return hits[a][0] < hits[c][0] })
	}
	return hits
}
