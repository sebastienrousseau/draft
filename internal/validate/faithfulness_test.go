// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/internal/claims"
)

func recs() []claims.Record {
	return []claims.Record{
		{Claim: "The model reached 0.82 on the test set", SourceQuote: "reached 0.82 on the test set", Type: "metric", Strength: "demonstrated"},
		{Claim: "Results may not generalise to larger models", SourceQuote: "may not generalise to larger models", Type: "limitation", Strength: "hedged"},
	}
}

func TestFaithfulnessClean(t *testing.T) {
	article := "The model reached 0.82 on the test set. It may not generalise to larger models."
	errs, _ := Faithfulness(article, recs())
	// No metric-term violation, ends on punctuation, no duplicates.
	for _, e := range errs {
		if strings.Contains(e, "metric term") || strings.Contains(e, "truncated") {
			t.Errorf("unexpected error: %s", e)
		}
	}
}

func TestFaithfulnessMetricConversion(t *testing.T) {
	// "perplexity" appears in no claim -> flagged as a possible conversion.
	article := "The model achieved a perplexity of 12. It reached 0.82 on the test set."
	errs, _ := Faithfulness(article, recs())
	if !hasSubstr(errs, "perplexity") {
		t.Errorf("expected metric-conversion error, got %v", errs)
	}
}

func TestFaithfulnessTruncation(t *testing.T) {
	errs, _ := Faithfulness("A sentence that just stops and", recs())
	if !hasSubstr(errs, "truncated") {
		t.Errorf("expected truncation error, got %v", errs)
	}
}

func TestFaithfulnessDuplicateParagraphs(t *testing.T) {
	para := strings.Repeat("the routing gate keeps twelve of sixty four heads per token in this model design ", 3)
	article := para + "\n\n" + para + "\n\n" + para
	errs, _ := Faithfulness(article+".", recs())
	if !hasSubstr(errs, "duplicate") {
		t.Errorf("expected duplicate-paragraph error, got %v", errs)
	}
}

func TestFaithfulnessHedgeUpgradeWarning(t *testing.T) {
	// An assertive verb over a hedged claim's tokens -> warning.
	article := "The work demonstrates that it generalise to larger models beyond doubt."
	_, warnings := Faithfulness(article, recs())
	if !hasSubstr(warnings, "hedge upgrade") {
		t.Errorf("expected hedge-upgrade warning, got %v", warnings)
	}
}

func TestFaithfulnessUngroundedNumbers(t *testing.T) {
	article := "The model reached 0.82 on the test set, up from 999 last year."
	_, warnings := Faithfulness(article, recs())
	if !hasSubstr(warnings, "999") {
		t.Errorf("expected ungrounded-number warning, got %v", warnings)
	}
}

func TestWordCountAndArticleShape(t *testing.T) {
	if WordCount("one two three") != 3 {
		t.Error("word count wrong")
	}
	if !LooksLikeArticle("# Title\n\n## Section\n\ntext") {
		t.Error("should look like an article")
	}
	if LooksLikeArticle("no heading here") {
		t.Error("should not look like an article")
	}
}

func TestBannedPhrases(t *testing.T) {
	errs := Errors("# T\n\n<aside class=\"post-lead\"></aside>\n\nExecutive Summary\n\n## S\n\n" +
		strings.Repeat("word ", 600) + "at its core this is fine.")
	if !hasSubstr(errs, "banned phrases") {
		t.Errorf("expected banned phrase detection, got %v", errs)
	}
}

func hasSubstr(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestShinglesAndJaccard(t *testing.T) {
	if len(shingles(nil, 4)) != 0 {
		t.Error("empty words -> no shingles")
	}
	if len(shingles([]string{"a", "b"}, 4)) != 1 {
		t.Error("fewer than k words -> single shingle")
	}
	if jaccard(map[string]bool{}, map[string]bool{}) != 0 {
		t.Error("two empty sets -> 0")
	}
	if jaccard(map[string]bool{"x": true}, map[string]bool{"x": true}) != 1 {
		t.Error("identical sets -> 1")
	}
}

func TestWordBoundaryContains(t *testing.T) {
	// "ax" appears inside "taxi" (no boundary) then standalone (boundary).
	if !wordBoundaryContains("taxi and ax here", "ax") {
		t.Error("should find the word-boundary occurrence after skipping the embedded one")
	}
	if wordBoundaryContains("taxink", "ax") {
		t.Error("embedded-only occurrence should not match a word boundary")
	}
	if !wordBoundaryContains("f1 score", "f1") {
		t.Error("standalone term should match")
	}
}

func TestDuplicateParagraphsMany(t *testing.T) {
	para := strings.Repeat("the routing gate keeps twelve of sixty four heads per token in this careful design ", 2)
	article := strings.Repeat(para+"\n\n", 5) // 5 near-identical paragraphs
	errs := duplicateParagraphs(article)
	if !hasSubstr(errs, "paragraphs nearly duplicate") {
		t.Errorf("expected the aggregate duplicate message, got %v", errs)
	}
}
