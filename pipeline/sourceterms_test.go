// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/rules"
)

// A banned word the source itself uses is the source's term, not filler.
// Rewriting it renamed "DeepSeek Harness" to "DeepSeek Use" and turned the
// paper's "ReAct harness" into "ReAct use" in published drafts.
func TestFinalizeKeepsTermsTheSourceUses(t *testing.T) {
	records := []claims.Record{
		{Claim: "x", SourceQuote: "written by Yujia Qiao for the DeepSeek Harness team"},
		{Claim: "y", SourceQuote: "compared with a ReAct harness using the same agent LLM"},
		{Claim: "z", SourceQuote: "an even more seamless checkout process"},
	}
	r := &Runner{}
	r.adoptSourceTerms(rules.DefaultStyle(), records)
	got := r.finalize("The DeepSeek Harness team built a ReAct harness for a more seamless flow, to leverage it.")
	for _, want := range []string{"DeepSeek Harness", "ReAct harness", "seamless"} {
		if !strings.Contains(got, want) {
			t.Errorf("source term %q was rewritten: %q", want, got)
		}
	}
	if strings.Contains(got, "leverage") {
		t.Errorf("a banned word the source does not use must still be rewritten: %q", got)
	}
}

// The validator must accept what the rewrite keeps, or a faithful draft fails
// its banned-vocabulary check and is sent back for a pointless retry.
func TestAdoptSourceTermsExemptsOnlyWhatTheSourceUses(t *testing.T) {
	records := []claims.Record{{Claim: "x", SourceQuote: "Harnesses, AT ITS CORE, are robust."}}
	r := &Runner{}
	style := r.adoptSourceTerms(rules.DefaultStyle(), records)
	for _, gone := range []string{"harness", "robust"} {
		for _, w := range style.BannedWords {
			if w == gone {
				t.Errorf("%q is used by the source and must not stay banned", gone)
			}
		}
	}
	for _, p := range style.BannedPhrases {
		if p == "at its core" {
			t.Error(`"at its core" is used by the source and must not stay banned`)
		}
	}
	kept := false
	for _, w := range style.BannedWords {
		kept = kept || w == "leverage"
	}
	if !kept {
		t.Error("a banned word the source does not use must stay banned")
	}
}
