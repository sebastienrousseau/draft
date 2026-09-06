// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/rules"
)

// A well-formed body with one word swapped in and out of the banned list shows
// that ErrorsWithStyle enforces the style's vocabulary, not the built-in one.
func styledArticle(body string) string {
	return "# Title\n\n**Thesis.**\n\n" +
		`<aside class="post-lead">TL;DR</aside>` + "\n\n" +
		"> Executive Summary\n\n## A heading\n\n" +
		strings.Repeat("This sentence carries real weight and reads plainly. ", 120) + body
}

func TestErrorsWithStyleUsesTheStyleVocabulary(t *testing.T) {
	body := styledArticle("The team will foobar the results.")
	// Default style does not ban "foobar".
	if hasWord(Errors(body), "banned word") {
		t.Error("default style should not flag foobar")
	}
	// A style that bans it does.
	custom := rules.DefaultStyle()
	custom.BannedWords = append(custom.BannedWords, "foobar")
	if !hasWord(ErrorsWithStyle(body, custom), "banned word") {
		t.Errorf("custom banned word not enforced: %v", ErrorsWithStyle(body, custom))
	}
	// A default banned word is NOT flagged under a style that replaces the list.
	leveraged := styledArticle("We leverage the results here.")
	if !hasWord(Errors(leveraged), "banned word") {
		t.Error("default style should flag leverage")
	}
	noWords := rules.DefaultStyle()
	noWords.BannedWords = nil
	if hasWord(ErrorsWithStyle(leveraged, noWords), "banned word") {
		t.Errorf("a style that bans no words must flag none: %v", ErrorsWithStyle(leveraged, noWords))
	}
}

func TestErrorsWithStyleWordBand(t *testing.T) {
	short := styledArticle("A short close.")
	tight := rules.DefaultStyle()
	tight.MinWords = 5000
	found := false
	for _, e := range ErrorsWithStyle(short, tight) {
		if strings.Contains(e, "minimum is 5000") {
			found = true
		}
	}
	if !found {
		t.Errorf("a raised minimum should be enforced: %v", ErrorsWithStyle(short, tight))
	}
}

func hasWord(errs []string, sub string) bool {
	for _, e := range errs {
		if strings.Contains(e, sub) {
			return true
		}
	}
	return false
}

// sameStrings distinguishes lists that differ only in one element, so a style
// with the default's length but a swapped word takes the compiled-on-demand
// path rather than reusing the default regex.
func TestErrorsWithStyleSwappedWordSameLength(t *testing.T) {
	custom := rules.DefaultStyle()
	custom.BannedWords = append([]string(nil), rules.BannedWords...)
	custom.BannedWords[0] = "flibbertigibbet" // same count, one word changed
	body := styledArticle("The report will delve into detail.")
	// "delve" was the default's first word; the swapped style no longer bans it.
	if hasWord(ErrorsWithStyle(body, custom), "banned word") {
		t.Errorf("swapped style must not enforce the removed default word: %v", ErrorsWithStyle(body, custom))
	}
}
