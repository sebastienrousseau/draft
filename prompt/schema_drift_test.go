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
