// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/engine"
)

// A continuation writes new article text, so it is held to the same claims as
// the first draft. Given only the draft's tail, a model finishing a truncated
// article has nothing to ground what it adds, and only the number checks stand
// between it and the published page.
func TestContinuationIsGivenTheVerifiedClaims(t *testing.T) {
	cases := map[string]struct {
		retries int
		writer  func(int) (string, bool)
	}{
		"first draft truncated": {0, func(call int) (string, bool) {
			if call == 1 {
				return validArticle(" and then"), true
			}
			return " it ended.", false
		}},
		"retry truncated": {1, func(call int) (string, bool) {
			switch call {
			case 1:
				return "# Too Short\n\nNot an article.", false
			case 2:
				return validArticle(" and then"), true
			}
			return " it ended.", false
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := testConfig(t)
			cfg.WriteRetries = tc.retries
			eng := &fakeEngine{name: "fake", writer: tc.writer}
			drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})

			continuation := eng.lastWritePrompt
			if !strings.Contains(continuation, "Continue the Markdown article") {
				t.Fatalf("the last write call was not a continuation:\n%.300s", continuation)
			}
			if !strings.Contains(continuation, "reached a score of 0.82 on the test set") {
				t.Errorf("the continuation prompt carries no verified claim:\n%s", continuation)
			}
			if !strings.Contains(continuation, "BEGIN UNTRUSTED VERIFIED CLAIMS") {
				t.Error("the claims in a continuation prompt must be fenced as untrusted")
			}
		})
	}
}
