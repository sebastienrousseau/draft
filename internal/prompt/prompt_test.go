// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package prompt

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/internal/rules"
)

func TestEffectiveStyleFallsBackToBuiltIn(t *testing.T) {
	if got := EffectiveStyle("   "); got != defaultStyleExample {
		t.Error("blank templates should yield the built-in style example")
	}
	if got := EffectiveStyle("MY TEMPLATE"); got != "MY TEMPLATE" {
		t.Error("provided templates should be returned unchanged")
	}
}

func TestDefaultStyleExampleHasNoCopyableHeadings(t *testing.T) {
	// The built-in style example must describe heading style as a principle, not
	// show concrete section headings a literal model would copy verbatim.
	for _, leak := range []string{"## What the result", "## Why the mechanism", "## Where it breaks"} {
		if strings.Contains(defaultStyleExample, leak) {
			t.Errorf("built-in style example still exposes a copyable heading: %q", leak)
		}
	}
	if !strings.Contains(defaultStyleExample, "Invent them from the claims") {
		t.Error("built-in style example should state the headings principle")
	}
}

func TestSkeletonMatchesStructureMarkers(t *testing.T) {
	// The output skeleton must embed exactly the markers the validator checks for,
	// so the template and validator cannot drift apart.
	for _, m := range []string{rules.PostLeadAsideMarker, rules.ExecSummaryMarker, rules.H1Prefix, rules.H2Prefix} {
		if !strings.Contains(outputSkeleton, m) {
			t.Errorf("output skeleton is missing the structural marker %q", m)
		}
	}
}

func TestClaimPromptContainsSource(t *testing.T) {
	p := Claim("SOME SOURCE TEXT")
	if !strings.Contains(p, "SOME SOURCE TEXT") {
		t.Error("claim prompt must embed the source")
	}
	if !strings.Contains(p, "SOURCE_QUOTE") {
		t.Error("claim prompt must define the record format")
	}
}

func TestWritingPromptUsesDefaultStyleWhenEmpty(t *testing.T) {
	p := Writing("", "LEDGER-CONTENT", rules.MinWords, rules.MaxWords)
	if !strings.Contains(p, "house style") {
		t.Error("empty templates should fall back to the built-in style example")
	}
	if !strings.Contains(p, "LEDGER-CONTENT") {
		t.Error("writing prompt must embed the ledger")
	}
	// Banned words the validator enforces must be named in the prompt.
	if !strings.Contains(p, rules.BannedWords[0]) {
		t.Error("writing prompt should list banned words")
	}
}

func TestWritingPromptUsesProvidedTemplates(t *testing.T) {
	p := Writing("MY TEMPLATE BLOCK", "L", rules.MinWords, rules.MaxWords)
	if !strings.Contains(p, "MY TEMPLATE BLOCK") {
		t.Error("provided templates should be embedded")
	}
	if strings.Contains(p, "house style") {
		t.Error("built-in style should not be used when templates are provided")
	}
}

func TestContinueWritingClipsTail(t *testing.T) {
	long := strings.Repeat("x", 9000)
	c := ContinueWriting(long)
	if len(c) > 5000 {
		t.Errorf("continuation prompt should clip the tail, len=%d", len(c))
	}
	if !strings.Contains(c, "Continue the Markdown article") {
		t.Error("missing continuation instruction")
	}
}

func TestReviewPrompt(t *testing.T) {
	long := strings.Repeat("r", MaxReviewSourceChars+100)
	p := Review(long, "DRAFT-BODY", "LEDGER-CONTENT")
	if !strings.Contains(p, "DRAFT-BODY") || !strings.Contains(p, "LEDGER-CONTENT") || !strings.Contains(p, "JSON array") {
		t.Error("review prompt missing draft, ledger, or output spec")
	}
	// clip applied: a contiguous run of exactly the cap, never one longer.
	if !strings.Contains(p, strings.Repeat("r", MaxReviewSourceChars)) || strings.Contains(p, strings.Repeat("r", MaxReviewSourceChars+1)) {
		t.Error("research should be clipped to MaxReviewSourceChars")
	}
	// short inputs pass through unclipped (the other clip branch).
	if !strings.Contains(Review("short research", "d", "l"), "short research") {
		t.Error("short research should not be clipped")
	}
}
