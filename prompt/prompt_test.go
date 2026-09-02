// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package prompt

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sebastienrousseau/draft/rules"
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

// A half rune normalises to U+FFFD, and claims.Verify rejects any quote that
// contains one — so a carelessly clipped prompt silently costs verified claims.
func TestPromptsStayValidUTF8WhenClipped(t *testing.T) {
	// 3-byte runes, so cuts at 4000 and 6000 bytes land mid-rune for some
	// prefix lengths. Sweep the offsets rather than trusting one alignment.
	for pad := 0; pad < 3; pad++ {
		body := strings.Repeat("x", pad) + strings.Repeat("字", 4000)

		if out := ContinueWriting(body); !utf8.ValidString(out) {
			t.Errorf("ContinueWriting(pad=%d) produced invalid UTF-8", pad)
		}
		if out := Review(body, "draft", "ledger"); !utf8.ValidString(out) {
			t.Errorf("Review(research, pad=%d) produced invalid UTF-8", pad)
		}
		if out := Review("research", body, "ledger"); !utf8.ValidString(out) {
			t.Errorf("Review(draft, pad=%d) produced invalid UTF-8", pad)
		}
	}
}

// Cached extractions are addressed partly by this value, so it must not vary
// between calls — the per-call nonce in the untrusted block would do exactly
// that if it were part of the hash.
func TestClaimVersionIsStableAndPromptSpecific(t *testing.T) {
	first := ClaimVersion()
	if first == "" {
		t.Fatal("ClaimVersion() is empty")
	}
	for i := 0; i < 5; i++ {
		if got := ClaimVersion(); got != first {
			t.Fatalf("ClaimVersion() varies between calls: %q then %q", first, got)
		}
	}
	// The source text is not part of the version: two different papers must
	// share a prompt version, or the cache key would double-count the body.
	if !strings.Contains(Claim("body one"), "body one") || !strings.Contains(Claim("body two"), "body two") {
		t.Fatal("Claim() no longer embeds its source")
	}
}
