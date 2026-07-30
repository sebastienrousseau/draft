// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/rules"
	"github.com/sebastienrousseau/draft/validate"
)

// editsJSON renders surgical edits the way a model returns them.
func editsJSON(t *testing.T, edits ...surgicalEdit) string {
	t.Helper()
	b, err := json.Marshal(edits)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The review path used to gate on validate.Errors alone, so --review could
// splice an ungrounded number into a draft and save it. It must gate on
// faithfulness too, exactly as the write path does — especially since
// "factual correction" is an allowed edit reason.
func TestReviewRejectsAnUngroundedEdit(t *testing.T) {
	cfg := testConfig(t)
	draft := writeDraft(t)
	before, err := os.ReadFile(draft)
	if err != nil {
		t.Fatal(err)
	}

	// The ledger supports "a score" but nothing about a metric term. The edit
	// introduces one, which only Faithfulness can catch.
	eng := &fakeEngine{name: "fake", editResponse: editsJSON(t, surgicalEdit{
		Find:    "The system reached a score on the test set.",
		Replace: "The system reached an F1 of 0.99 on the test set.",
		Reason:  "factual correction",
	})}

	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}, ReviewPath: draft})
	if errText == "" {
		t.Fatal("an ungrounded edit was accepted; the review path is not checking faithfulness")
	}
	if !strings.Contains(errText, "broke the rules") {
		t.Errorf("unexpected failure text: %s", errText)
	}

	// A rejected review must leave the user's draft exactly as it was.
	after, err := os.ReadFile(draft)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Error("the draft was modified despite the review failing")
	}
}

// The gate must not be so blunt that a legitimate edit cannot pass.
func TestReviewAcceptsAGroundedEdit(t *testing.T) {
	cfg := testConfig(t)
	draft := writeDraft(t)

	eng := &fakeEngine{name: "fake", editResponse: editsJSON(t, surgicalEdit{
		Find:    "A single number tells the story.",
		Replace: "One number tells the story.",
		Reason:  "generic",
	})}

	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}, ReviewPath: draft})
	if errText != "" {
		t.Fatalf("a grounded style edit should be accepted: %s", errText)
	}
	data, err := os.ReadFile(draft)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "One number tells the story.") {
		t.Error("the accepted edit was not applied")
	}
}

// Both write paths must run both gates. This is the parity assertion that
// would have caught the review path skipping Faithfulness in the first place:
// any article the write path would reject must also be rejected on review.
func TestBothWritePathsApplyTheSameGates(t *testing.T) {
	ledger := []claims.Record{{
		Claim:       "The system reached a score of 0.82 on the test set",
		SourceQuote: "reached a score of 0.82 on the test set",
		Type:        "metric",
		Strength:    "demonstrated",
	}}
	ungrounded := validArticle(" The system reached an F1 of 0.99.")

	styleErrs := validate.Errors(ungrounded)
	factErrs, _ := validate.Faithfulness(ungrounded, ledger)
	if len(factErrs) == 0 {
		t.Fatal("fixture is wrong: the ungrounded article must fail Faithfulness")
	}
	if len(styleErrs) != 0 {
		t.Fatalf("fixture is wrong: it must pass the style rules, got %v", styleErrs)
	}
	// Style checks alone would have let this through — which is precisely the
	// hole the review path used to have.
}

// A model that returns something other than an edit array must fail closed and
// leave the draft untouched.
func TestReviewRejectsMalformedEdits(t *testing.T) {
	cfg := testConfig(t)
	draft := writeDraft(t)
	before, err := os.ReadFile(draft)
	if err != nil {
		t.Fatal(err)
	}

	eng := &fakeEngine{name: "fake", editResponse: "I have decided not to answer."}
	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}, ReviewPath: draft})
	if errText == "" {
		t.Fatal("expected the review to fail on unparseable output")
	}

	after, err := os.ReadFile(draft)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Error("the draft was modified despite the edits being unparseable")
	}
}

// An edit whose Find is not unique cannot be located safely, so it must be
// refused rather than applied to the first match.
func TestReviewRejectsAmbiguousEdit(t *testing.T) {
	cfg := testConfig(t)
	draft := writeDraft(t)

	eng := &fakeEngine{name: "fake", editResponse: editsJSON(t, surgicalEdit{
		Find:    "The grounded result stands on its own", // appears 110 times
		Replace: "Something else",
		Reason:  "generic",
	})}

	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}, ReviewPath: draft})
	if !strings.Contains(errText, "expected 1") {
		t.Errorf("expected a uniqueness failure, got %q", errText)
	}
}

func TestReviewDoneEventCarriesTimings(t *testing.T) {
	cfg := testConfig(t)
	draft := writeDraft(t)
	eng := &fakeEngine{name: "fake"}

	done, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}, ReviewPath: draft})
	if errText != "" {
		t.Fatalf("review failed: %s", errText)
	}
	if done.Mode != "review" {
		t.Errorf("Mode = %q, want review", done.Mode)
	}
	if done.Duration <= 0 {
		t.Error("DoneEvent.Duration was not set on the review path")
	}
	if len(done.Timings) == 0 {
		t.Error("DoneEvent.Timings was empty on the review path")
	}
}

var _ = engine.KindEdit

// longArticle builds a structurally valid draft that exceeds rules.MaxWords.
func longArticle(t *testing.T) string {
	t.Helper()
	body := strings.Repeat("The grounded result stands on its own and reads plainly. ", 400)
	md := "# The Result That Holds\n\n" +
		"**A single number tells the story.**\n\n" +
		`<aside class="post-lead"><p><strong>TL;DR.</strong> A grounded look.</p></aside>` + "\n\n" +
		"> **Executive Summary**\n>\n> - The system reached a score on the test set.\n\n" +
		"## What the result shows\n\n" + body + "."
	if got := validate.WordCount(md); got <= rules.MaxWords {
		t.Fatalf("fixture is wrong: %d words does not exceed the %d cap", got, rules.MaxWords)
	}
	return md
}

// --review must judge the edit, not the article it was applied to. An article
// the user already has may predate a rule or exceed the length band; failing on
// that makes it permanently unreviewable for a reason the review had nothing to
// do with. This is the regression guard for enforcing rules.MaxWords.
func TestReviewToleratesAPreExistingViolation(t *testing.T) {
	cfg := testConfig(t)
	dir := t.TempDir()
	draft := filepath.Join(dir, "long.md")
	if err := os.WriteFile(draft, []byte(longArticle(t)), 0o644); err != nil {
		t.Fatal(err)
	}

	eng := &fakeEngine{name: "fake", editResponse: editsJSON(t, surgicalEdit{
		Find:    "A single number tells the story.",
		Replace: "One number tells the story.",
		Reason:  "generic",
	})}

	_, errText, logs := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}, ReviewPath: draft})
	if errText != "" {
		t.Fatalf("an over-long article must still be reviewable: %s", errText)
	}
	data, err := os.ReadFile(draft)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "One number tells the story.") {
		t.Error("the edit was not applied")
	}
	// The user should still be told about it.
	var reported bool
	for _, l := range logs {
		if strings.Contains(l, "pre-existing") && strings.Contains(l, "maximum") {
			reported = true
		}
	}
	if !reported {
		t.Errorf("a pre-existing violation should be surfaced as a warning, got %v", logs)
	}
}

// An edit that makes a pre-existing problem worse is still the edit's fault.
func TestReviewStillRejectsAViolationItIntroduces(t *testing.T) {
	cfg := testConfig(t)
	dir := t.TempDir()
	draft := filepath.Join(dir, "long.md")
	if err := os.WriteFile(draft, []byte(longArticle(t)), 0o644); err != nil {
		t.Fatal(err)
	}

	// The article is already over length; this edit additionally breaks the
	// title, which is a violation the original did not have.
	eng := &fakeEngine{name: "fake", editResponse: editsJSON(t, surgicalEdit{
		Find:    "# The Result That Holds",
		Replace: "Not a heading anymore",
		Reason:  "generic",
	})}

	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}, ReviewPath: draft})
	if !strings.Contains(errText, "broke the rules") {
		t.Fatalf("expected the introduced violation to be rejected, got %q", errText)
	}
	if strings.Contains(errText, "maximum") {
		t.Errorf("the pre-existing length violation should not be blamed on the edit: %q", errText)
	}
}
