// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package prompt

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/claims"
)

// The JSON variant goes to an engine that constrains its output to
// claims.Schema(). It must ask for that shape, not the CLAIM: text records,
// or a constrained model is told one format and forced into another.
func TestClaimJSONAsksForTheSchemaShape(t *testing.T) {
	p := ClaimJSON("the body")
	if !strings.Contains(p, "the body") {
		t.Fatal("ClaimJSON() does not embed its source")
	}
	for _, field := range []string{`"claims"`, `"claim"`, `"source_quote"`, `"type"`, `"strength"`} {
		if !strings.Contains(p, field) {
			t.Errorf("JSON claim prompt does not name the %s field", field)
		}
	}
	for _, v := range append(append([]string{}, claims.ClaimTypes...), claims.ClaimStrengths...) {
		if !strings.Contains(p, v) {
			t.Errorf("JSON claim prompt does not mention %q", v)
		}
	}
	if strings.Contains(p, "CLAIM: <") || strings.Contains(p, "return exactly: NONE") {
		t.Error("JSON claim prompt still asks for the CLAIM: text format")
	}
	if !strings.Contains(p, "UNTRUSTED SOURCE DOCUMENT") {
		t.Error("JSON claim prompt does not fence its source as untrusted")
	}
}

// The rules a model is held to must be the same in both variants: only the
// output section differs. Otherwise switching an engine to structured output
// would quietly change what it is asked to extract.
func TestClaimJSONSharesTheRules(t *testing.T) {
	rules := func(p string) string {
		start := strings.Index(p, "## RULES")
		end := strings.Index(p, "## OUTPUT")
		if start < 0 || end < start {
			t.Fatalf("prompt has no RULES section before OUTPUT:\n%s", p)
		}
		return p[start:end]
	}
	if rules(Claim("x")) != rules(ClaimJSON("x")) {
		t.Error("the JSON claim prompt's RULES differ from the text prompt's")
	}
}

// Cached extractions are keyed by prompt version, and the two variants
// produce different output, so they must not share one.
func TestClaimJSONVersionIsStableAndDistinct(t *testing.T) {
	v := ClaimJSONVersion()
	if v == "" || v != ClaimJSONVersion() {
		t.Fatalf("ClaimJSONVersion() is empty or unstable: %q", v)
	}
	if v == ClaimVersion() {
		t.Error("the JSON and text claim prompts share a version")
	}
}
