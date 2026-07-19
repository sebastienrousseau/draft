package prompt

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/internal/rules"
)

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
	p := Writing("", "LEDGER-CONTENT")
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
	p := Writing("MY TEMPLATE BLOCK", "L")
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

func TestReviewPromptClipsInputs(t *testing.T) {
	research := strings.Repeat("r", MaxReviewSourceChars+1000)
	draft := strings.Repeat("d", MaxDraftChars+1000)
	p := Review(research, draft, "LEDGER")
	// The clipped inputs leave a contiguous run of exactly the cap length, never
	// one character longer.
	if !strings.Contains(p, strings.Repeat("r", MaxReviewSourceChars)) || strings.Contains(p, strings.Repeat("r", MaxReviewSourceChars+1)) {
		t.Error("research should be clipped to MaxReviewSourceChars")
	}
	if !strings.Contains(p, strings.Repeat("d", MaxDraftChars)) || strings.Contains(p, strings.Repeat("d", MaxDraftChars+1)) {
		t.Error("draft should be clipped to MaxDraftChars")
	}
	if !strings.Contains(p, "LEDGER") || !strings.Contains(p, "JSON array") {
		t.Error("review prompt missing ledger or output spec")
	}
}

func TestReviewShortInputsNoClip(t *testing.T) {
	p := Review("short research", "short draft", "LEDGER")
	if !strings.Contains(p, "short research") || !strings.Contains(p, "short draft") {
		t.Error("short inputs should pass through unclipped")
	}
}
