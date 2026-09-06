// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package rules

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Style is the editorial policy a run enforces: the word band, the banned
// vocabulary, and the language variant. The defaults are this project's house
// style; a Style loaded from a file replaces or extends them, so a different
// publication can use draft's grounding without adopting its voice.
//
// Only what a publication actually changes lives here. The structural rules —
// an H1, a lead aside, an executive summary, section headings — are the shape
// of a grounded article rather than a matter of taste, and stay fixed.
type Style struct {
	MinWords      int      `json:"min_words"`
	MaxWords      int      `json:"max_words"`
	BannedWords   []string `json:"banned_words"`
	BannedPhrases []string `json:"banned_phrases"`
	// English is the language-variant instruction handed to the writer, e.g.
	// "British English" or "American English". Empty means no instruction.
	English string `json:"english"`
}

// DefaultStyle is draft's own house style: the values the prompt and validator
// used before the style file existed. A loaded style starts from this.
func DefaultStyle() Style {
	return Style{
		MinWords:      MinWords,
		MaxWords:      MaxWords,
		BannedWords:   append([]string(nil), BannedWords...),
		BannedPhrases: append([]string(nil), BannedPhrases...),
		English:       "British English",
	}
}

// OrDefault returns DefaultStyle when the receiver is a zero value, so a
// Config built as a literal (in a test, or by an embedding caller) still has a
// usable style without every such caller remembering to set one.
func (s Style) OrDefault() Style {
	if s.MaxWords == 0 && s.MinWords == 0 && s.BannedWords == nil {
		return DefaultStyle()
	}
	return s
}

// BannedWordForms expands the style's banned words to their inflections, the
// set the validator matches on.
func (s Style) BannedWordForms() []string {
	var forms []string
	for _, w := range s.BannedWords {
		for _, f := range WordForms(w) {
			forms = append(forms, f.Form)
		}
	}
	return forms
}

// styleFile is the on-disk shape. Replacement fields set a list outright;
// the "also_" fields add to the default, which is what a publication that
// wants draft's clichés plus a few of its own reaches for. A replacement and
// its matching addition compose: the replacement wins the base, the addition
// extends it.
type styleFile struct {
	MinWords          *int     `json:"min_words"`
	MaxWords          *int     `json:"max_words"`
	English           *string  `json:"english"`
	BannedWords       []string `json:"banned_words"`
	BannedPhrases     []string `json:"banned_phrases"`
	AlsoBannedWords   []string `json:"also_banned_words"`
	AlsoBannedPhrases []string `json:"also_banned_phrases"`
}

// LoadStyle reads a JSON style file and returns it merged onto DefaultStyle,
// so a file that sets only one field keeps the defaults for the rest. An
// unknown field is an error, because a misspelled key that was silently
// ignored would leave the writer held to a rule the author thought they had
// changed.
func LoadStyle(path string) (Style, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Style{}, err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	var f styleFile
	if err := dec.Decode(&f); err != nil {
		return Style{}, fmt.Errorf("%s: %w", path, err)
	}

	s := DefaultStyle()
	if f.MinWords != nil {
		s.MinWords = *f.MinWords
	}
	if f.MaxWords != nil {
		s.MaxWords = *f.MaxWords
	}
	if f.English != nil {
		s.English = strings.TrimSpace(*f.English)
	}
	if f.BannedWords != nil {
		s.BannedWords = f.BannedWords
	}
	if f.BannedPhrases != nil {
		s.BannedPhrases = f.BannedPhrases
	}
	s.BannedWords = mergeLower(s.BannedWords, f.AlsoBannedWords)
	s.BannedPhrases = mergeLower(s.BannedPhrases, f.AlsoBannedPhrases)

	if err := s.validate(); err != nil {
		return Style{}, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// validate rejects a style that could not admit any article. A band that is
// empty or inverted would fail every draft with a word-count error the author
// could never satisfy.
func (s Style) validate() error {
	if s.MinWords < 1 {
		return fmt.Errorf("min_words must be at least 1, got %d", s.MinWords)
	}
	if s.MaxWords < s.MinWords {
		return fmt.Errorf("max_words (%d) must be at least min_words (%d)", s.MaxWords, s.MinWords)
	}
	return nil
}

// mergeLower appends additions to base, lower-casing and de-duplicating so the
// banned lists compare against lowered article text without surprises.
func mergeLower(base, add []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(base)+len(add))
	for _, w := range append(append([]string(nil), base...), add...) {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}
