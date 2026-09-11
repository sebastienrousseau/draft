// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package prompt

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/claims"
)

// The extraction prompt spells out the allowed TYPE and STRENGTH values, and
// claims.Schema() encodes the same set for structured decoding. If the two
// drift, a constrained model and the prompt would disagree about the vocabulary.
// This pins them together: every canonical value must appear in the prompt.
func TestClaimPromptListsCanonicalEnums(t *testing.T) {
	p := Claim("The system reached 99 pages per second.")
	for _, v := range claims.ClaimTypes {
		if !strings.Contains(p, v) {
			t.Errorf("claim prompt does not mention TYPE value %q", v)
		}
	}
	for _, v := range claims.ClaimStrengths {
		if !strings.Contains(p, v) {
			t.Errorf("claim prompt does not mention STRENGTH value %q", v)
		}
	}
}

func TestEntailmentPromptShape(t *testing.T) {
	p := Entailment("The method improves accuracy.", "the method improves accuracy across all benchmarks")
	// The claim and the (fenced) quote are both present, and the model is asked
	// for the one-word verdict the parser reads.
	for _, want := range []string{
		"The method improves accuracy.",
		"improves accuracy across all benchmarks",
		"SUPPORTED",
		"UNSUPPORTED",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("entailment prompt missing %q", want)
		}
	}
	// The quote is wrapped in the untrusted fence, not pasted raw as an
	// instruction.
	if !strings.Contains(p, "UNTRUSTED SOURCE QUOTE") {
		t.Errorf("quote is not fenced as untrusted:\n%s", p)
	}
}
