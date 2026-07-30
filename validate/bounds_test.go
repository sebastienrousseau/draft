// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/rules"
)

// article builds a structurally valid draft of roughly n words.
func article(n int) string {
	head := "# A Title That Holds\n\n" +
		"**One number tells the story.**\n\n" +
		`<aside class="post-lead"><p><strong>TL;DR.</strong> A grounded look.</p></aside>` + "\n\n" +
		"> **Executive Summary**\n>\n> - The system reached a score.\n\n" +
		"## What it shows\n\n"
	// Eight words per repetition, then a closing full stop.
	reps := n / 8
	return head + strings.Repeat("the grounded result stands on its own plainly ", reps) + "."
}

// The writing prompt asks for MinWords–MaxWords and rules declares that band
// for a finished draft, but only the floor was ever enforced — so a runaway
// draft was told one thing and held to another.
func TestErrorsEnforcesBothWordBounds(t *testing.T) {
	for _, tc := range []struct {
		name      string
		words     int
		wantError string
	}{
		{name: "below the floor", words: 40, wantError: "minimum"},
		{name: "comfortably inside", words: 900},
		{name: "just inside the ceiling", words: rules.MaxWords - 200},
		{name: "past the ceiling", words: rules.MaxWords + 400, wantError: "maximum"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			md := article(tc.words)
			got := WordCount(md)

			var wordErr string
			for _, e := range Errors(md) {
				if strings.Contains(e, "words") {
					wordErr = e
				}
			}

			if tc.wantError == "" {
				if wordErr != "" {
					t.Errorf("a %d-word draft was rejected: %s", got, wordErr)
				}
				return
			}
			if wordErr == "" {
				t.Fatalf("a %d-word draft was accepted; expected a %q violation", got, tc.wantError)
			}
			if !strings.Contains(wordErr, tc.wantError) {
				t.Errorf("error = %q, want it to mention %q", wordErr, tc.wantError)
			}
		})
	}
}

// The two bounds are mutually exclusive: a draft cannot be reported as both.
func TestErrorsReportsOnlyOneWordBound(t *testing.T) {
	for _, words := range []int{40, 900, rules.MaxWords + 400} {
		count := 0
		for _, e := range Errors(article(words)) {
			if strings.Contains(e, "words;") {
				count++
			}
		}
		if count > 1 {
			t.Errorf("%d-word draft produced %d word-count errors", words, count)
		}
	}
}
