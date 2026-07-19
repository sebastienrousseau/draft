// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/internal/engine"
)

func writeDraft(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "draft.md")
	if err := os.WriteFile(path, []byte(validArticle(".")), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReviewAppliesEdits(t *testing.T) {
	cfg := testConfig(t)
	draft := writeDraft(t)
	eng := &fakeEngine{
		name:         "fake",
		editResponse: `[{"find":"A single number tells the story.","replace":"A single number tells the whole story.","reason":"generic"}]`,
	}
	done, errText, logs := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}, ReviewPath: draft})
	if errText != "" {
		t.Fatalf("review should succeed: %s", errText)
	}
	if done.Mode != "review" || done.OutputPath != draft {
		t.Errorf("unexpected done event: %+v", done)
	}
	data, _ := os.ReadFile(draft)
	if !strings.Contains(string(data), "whole story") {
		t.Error("surgical edit was not applied to the draft")
	}
	if !hasLog(logs, "applied 1 surgical edit") {
		t.Errorf("expected an applied-edits log, got %v", logs)
	}
}

func TestReviewNoEditsIsNoop(t *testing.T) {
	cfg := testConfig(t)
	draft := writeDraft(t)
	before, _ := os.ReadFile(draft)
	// default editResponse "[]" -> no edits.
	done, errText, _ := drain(t, cfg, []engine.Engine{&fakeEngine{name: "fake"}}, Job{Sources: []string{writeSource(t)}, ReviewPath: draft})
	if errText != "" {
		t.Fatalf("empty edits should still succeed: %s", errText)
	}
	if done.Mode != "review" {
		t.Errorf("mode = %q, want review", done.Mode)
	}
	after, _ := os.ReadFile(draft)
	if strings.TrimRight(string(after), "\n") != strings.TrimRight(string(before), "\n") {
		t.Error("no-op review should not change the draft body")
	}
}

func TestReviewInvalidEditsFails(t *testing.T) {
	cfg := testConfig(t)
	draft := writeDraft(t)
	// A find that occurs more than once (the repeated body sentence) is rejected.
	eng := &fakeEngine{name: "fake", editResponse: `[{"find":"The grounded result stands on its own and reads plainly. ","replace":"x","reason":"filler"}]`}
	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}, ReviewPath: draft})
	if !strings.Contains(errText, "surgical edits failed") {
		t.Errorf("expected a surgical-edit failure, got %q", errText)
	}
}

func TestReviewMissingDraft(t *testing.T) {
	cfg := testConfig(t)
	_, errText, _ := drain(t, cfg, []engine.Engine{&fakeEngine{name: "fake"}}, Job{Sources: []string{writeSource(t)}, ReviewPath: "/no/such/draft.md"})
	if !strings.Contains(errText, "could not read draft") {
		t.Errorf("expected a read error, got %q", errText)
	}
}

func TestParseSurgicalEdits(t *testing.T) {
	edits, err := parseSurgicalEdits("prelude text [\n{\"find\":\"a\",\"replace\":\"b\",\"reason\":\"generic\"}\n] trailing")
	if err != nil || len(edits) != 1 || edits[0].Find != "a" {
		t.Fatalf("parse failed: %v %+v", err, edits)
	}
	if _, err := parseSurgicalEdits("no array here"); err == nil {
		t.Error("expected error when no JSON array present")
	}
	// Chain-of-thought preamble is stripped.
	if e, err := parseSurgicalEdits("<think>reasoning</think>[]"); err != nil || len(e) != 0 {
		t.Errorf("think-stripped empty array: %v %+v", err, e)
	}
}

func TestApplySurgicalEdits(t *testing.T) {
	src := "The quick brown fox jumps over the lazy dog."
	out, err := applySurgicalEdits(src, []surgicalEdit{{Find: "quick", Replace: "slow", Reason: "generic"}})
	if err != nil || out != "The slow brown fox jumps over the lazy dog." {
		t.Fatalf("apply failed: %v %q", err, out)
	}
	// Unsupported reason.
	if _, err := applySurgicalEdits(src, []surgicalEdit{{Find: "fox", Replace: "cat", Reason: "bogus"}}); err == nil {
		t.Error("expected error for unsupported reason")
	}
	// Non-unique find.
	if _, err := applySurgicalEdits("a a a", []surgicalEdit{{Find: "a", Replace: "b", Reason: "generic"}}); err == nil {
		t.Error("expected error for non-unique find")
	}
	// Overlapping edits.
	if _, err := applySurgicalEdits("abcdef", []surgicalEdit{
		{Find: "abc", Replace: "x", Reason: "generic"},
		{Find: "bcd", Replace: "y", Reason: "generic"},
	}); err == nil {
		t.Error("expected error for overlapping edits")
	}
	// Empty find.
	if _, err := applySurgicalEdits(src, []surgicalEdit{{Find: "", Replace: "x", Reason: "generic"}}); err == nil {
		t.Error("expected error for empty find")
	}
}

func TestReviewNoReadableSource(t *testing.T) {
	cfg := testConfig(t)
	draft := writeDraft(t)
	bad := filepath.Join(t.TempDir(), "x.xyz")
	os.WriteFile(bad, []byte("x"), 0o644)
	_, errText, _ := drain(t, cfg, []engine.Engine{&fakeEngine{name: "fake"}}, Job{Sources: []string{bad}, ReviewPath: draft})
	if !strings.Contains(errText, "no readable source") {
		t.Errorf("expected no-readable-source error, got %q", errText)
	}
}

func TestReviewInvalidJSON(t *testing.T) {
	cfg := testConfig(t)
	draft := writeDraft(t)
	eng := &fakeEngine{name: "fake", editResponse: "not a json array at all"}
	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}, ReviewPath: draft})
	if !strings.Contains(errText, "valid surgical edits") {
		t.Errorf("expected invalid-edits error, got %q", errText)
	}
}

func TestReviewBreaksRules(t *testing.T) {
	cfg := testConfig(t)
	draft := writeDraft(t)
	// Replacing the H1 with non-heading text makes the enhanced draft fail the
	// house rules, so it is rejected rather than saved.
	eng := &fakeEngine{name: "fake", editResponse: `[{"find":"# The Result That Holds","replace":"Not a heading anymore","reason":"factual correction"}]`}
	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}, ReviewPath: draft})
	if !strings.Contains(errText, "broke the rules") {
		t.Errorf("expected rule-break rejection, got %q", errText)
	}
}

func TestReviewExtractionFails(t *testing.T) {
	cfg := testConfig(t)
	draft := writeDraft(t)
	eng := &fakeEngine{name: "fake", failAll: errTest}
	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}, ReviewPath: draft})
	if !strings.Contains(errText, "claim extraction failed") {
		t.Errorf("expected extraction failure in review, got %q", errText)
	}
}

func TestReviewEditGenerationFails(t *testing.T) {
	cfg := testConfig(t)
	draft := writeDraft(t)
	eng := &fakeEngine{name: "fake", failEdit: true}
	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}, ReviewPath: draft})
	if !strings.Contains(errText, "review generation failed") {
		t.Errorf("expected review generation failure, got %q", errText)
	}
}
