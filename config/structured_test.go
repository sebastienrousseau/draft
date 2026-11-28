// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package config

import "testing"

// Structured extraction is not yet shown to be better than text, so it must
// stay off unless asked for, and DRAFT_STRUCTURED_OUTPUT=1 must turn it on.
func TestStructuredOutputIsOptIn(t *testing.T) {
	t.Setenv("DRAFT_NO_CONFIG", "1")
	t.Setenv("DRAFT_STRUCTURED_OUTPUT", "")
	if Load(Flags{}).StructuredOutput {
		t.Error("structured output is on by default")
	}
	t.Setenv("DRAFT_STRUCTURED_OUTPUT", "1")
	if !Load(Flags{}).StructuredOutput {
		t.Error("DRAFT_STRUCTURED_OUTPUT=1 did not turn structured output on")
	}
}
