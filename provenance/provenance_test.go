// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package provenance

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sebastienrousseau/draft/claims"
)

var ledger = []claims.Record{
	{Claim: "The system reached a score of 0.82 on the test set", SourceQuote: "reached a score of 0.82 on the test set", Type: "metric", Strength: "demonstrated"},
	{Claim: "Training used 5x fewer samples than the previous approach", SourceQuote: "used 5x fewer samples than before", Type: "metric", Strength: "demonstrated"},
	{Claim: "The agents coordinated through a shared message board", SourceQuote: "agents coordinated their lanes through a shared message board", Type: "mechanism", Strength: "demonstrated"},
}

const body = `# The Result That Holds

**A single number tells the story.**

<!-- lead-start -->
<aside class="post-lead" aria-label="Article summary">
<p class="post-lead-tldr"><strong>TL;DR.</strong> A grounded look.</p>
</aside>
<!-- lead-end -->

> **Executive Summary**
>
> - The system reached a score of 0.82 on the test set.

## What the numbers show

The system reached a score of 0.82 on the test set. It did so with 5x fewer samples than before, which matters.
Nothing here is invented! The agents coordinated their lanes through a shared message board.

| metric | value |
| ------ | ----- |
| score  | 0.82  |

- A list item that says the score was 0.82 on the test set.
- Throughput improved by 34% once the cache was enabled.

` + "```" + `
code with a number 999.
` + "```" + `

---

Framing that draws on nothing in particular.
`

func TestClaimIDIsStableAndQuoteDerived(t *testing.T) {
	a := ClaimID(claims.Record{SourceQuote: "Reached a score of 0.82  on the test set", Claim: "x"})
	b := ClaimID(claims.Record{SourceQuote: "reached a score of 0.82 on the test set", Claim: "y"})
	if a != b {
		t.Errorf("case and spacing must not change the identifier: %s vs %s", a, b)
	}
	if !strings.HasPrefix(a, "c") || len(a) != 11 {
		t.Errorf("identifier shape = %q", a)
	}
	if ClaimID(claims.Record{SourceQuote: "a different quote entirely"}) == a {
		t.Error("different quotes must not collide")
	}
	ids := Claims(ledger)
	if len(ids) != 3 || ids[0].ID != ClaimID(ledger[0]) || ids[2].Type != "mechanism" {
		t.Errorf("Claims() = %+v", ids)
	}
}

func TestAttributeMapsSentencesToClaims(t *testing.T) {
	att := Attribute(body, ledger)
	if att.Schema != Schema || att.ArticleSHA256 == "" || len(att.Claims) != 3 {
		t.Fatalf("header = %+v", att)
	}
	byText := map[string]Sentence{}
	for _, s := range att.Sentences {
		byText[s.Text] = s
		if body[s.Start:s.End] != s.Text {
			t.Errorf("offsets do not address the sentence: %q vs %q", body[s.Start:s.End], s.Text)
		}
	}
	score := ClaimID(ledger[0])
	samples := ClaimID(ledger[1])
	board := ClaimID(ledger[2])

	want := map[string]struct {
		claim, evidence string
	}{
		"The system reached a score of 0.82 on the test set.":                {score, "number"},
		"It did so with 5x fewer samples than before, which matters.":        {samples, "number"},
		"The agents coordinated their lanes through a shared message board.": {board, "lexical"},
		"A list item that says the score was 0.82 on the test set.":          {score, "number"},
		"**A single number tells the story.**":                               {"", ""},
		"Framing that draws on nothing in particular.":                       {"", ""},
	}
	for text, w := range want {
		s, ok := byText[text]
		if !ok {
			t.Errorf("sentence not found: %q (have %d sentences)", text, len(att.Sentences))
			continue
		}
		if w.claim == "" {
			if len(s.Claims) != 0 {
				t.Errorf("%q should be unattributed, got %v", text, s.Claims)
			}
			continue
		}
		if len(s.Claims) == 0 || s.Claims[0] != w.claim || s.Evidence != w.evidence {
			t.Errorf("%q -> %v (%s), want %s (%s)", text, s.Claims, s.Evidence, w.claim, w.evidence)
		}
	}
	// The executive summary bullet is a quote-marked list item and is prose.
	if s, ok := byText["The system reached a score of 0.82 on the test set."]; !ok || s.Index < 0 {
		t.Error("blockquoted list item should be attributed like any sentence")
	}
	// Structure is not a sentence: headings, HTML, table rows, rules, code.
	for _, s := range att.Sentences {
		for _, bad := range []string{"# The Result", "<aside", "| score", "```", "999", "---", "Executive Summary"} {
			if strings.Contains(s.Text, bad) {
				t.Errorf("structure leaked into sentences: %q", s.Text)
			}
		}
	}
	// A figure no claim has is flagged on its sentence and counted.
	s := byText["Throughput improved by 34% once the cache was enabled."]
	if len(s.UngroundedNumbers) != 1 || s.UngroundedNumbers[0] != "34" {
		t.Errorf("ungrounded numbers = %v", s.UngroundedNumbers)
	}
	if att.SentencesWithUngroundedNumbers != 1 {
		t.Errorf("sentences with ungrounded numbers = %d", att.SentencesWithUngroundedNumbers)
	}
	if att.Attributed+att.Unattributed != len(att.Sentences) || att.Attributed < 4 {
		t.Errorf("counts: attributed %d unattributed %d of %d", att.Attributed, att.Unattributed, len(att.Sentences))
	}
	if _, err := json.Marshal(att); err != nil {
		t.Fatal(err)
	}
}

func TestSentencesKeepDecimalsAndStripMarkers(t *testing.T) {
	got := sentences("> - The value was 0.82 today. Then 3.5 tomorrow!\n1. Ordered item? Yes.\n")
	texts := make([]string, 0, len(got))
	for _, s := range got {
		texts = append(texts, s.text)
	}
	want := []string{"The value was 0.82 today.", "Then 3.5 tomorrow!", "Ordered item?", "Yes."}
	if strings.Join(texts, "|") != strings.Join(want, "|") {
		t.Errorf("sentences = %q, want %q", texts, want)
	}
	if got[0].start != 4 {
		t.Errorf("first sentence starts at %d after '> - ', want 4", got[0].start)
	}
	if n := len(sentences("")); n != 0 {
		t.Errorf("empty body yields %d sentences", n)
	}
}

func TestAttributeCapsClaimsPerSentence(t *testing.T) {
	var many []claims.Record
	for i := 0; i < 6; i++ {
		many = append(many, claims.Record{Claim: "the score reached 0.82 on the test set", SourceQuote: strings.Repeat("x", i+1) + " the score reached 0.82 on the test set"})
	}
	att := Attribute("The score reached 0.82 on the test set.\n", many)
	if len(att.Sentences) != 1 || len(att.Sentences[0].Claims) != maxClaimsPerSentence {
		t.Errorf("claims per sentence = %d, want cap %d", len(att.Sentences[0].Claims), maxClaimsPerSentence)
	}
}

func TestManifestBindsEverything(t *testing.T) {
	att := Attribute(body, ledger)
	when := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	m := NewManifest(ManifestInput{
		Title: "The Result That Holds", Body: body, Version: "0.0.34", Engine: "claude", Model: "sonnet", Reader: "pdftotext",
		PromptVersion: "pv1", LedgerSHA256: "abc", When: when, Attribution: &att,
		Sources: []Source{{Path: "/papers/x.pdf", SHA256: "deadbeef"}, {Path: `C:\notes\memo.docx`, SHA256: "cafe"}, {Path: "notes.md", SHA256: "1"}, {Path: "plain.txt", SHA256: "2"}, {Path: "weird.bin", SHA256: "3"}},
	})
	if m.Title != "The Result That Holds" || m.Format != "text/markdown" || m.ClaimGeneratorInfo[0].Version != "0.0.34" {
		t.Errorf("header = %+v", m)
	}
	if len(m.Assertions) != 2 || m.Assertions[0].Label != "c2pa.actions" || m.Assertions[1].Label != GroundingLabel {
		t.Fatalf("assertions = %+v", m.Assertions)
	}
	g := m.Assertions[1].Data.(Grounding)
	if g.ArticleSHA256 != att.ArticleSHA256 || g.LedgerSHA256 != "abc" || g.Engine != "claude" || g.Reader != "pdftotext" || len(g.Claims) != 3 || g.Sentences != len(att.Sentences) || g.Attributed != att.Attributed {
		t.Errorf("grounding = %+v", g)
	}
	if len(m.Ingredients) != 5 || m.Ingredients[0].Title != "x.pdf" || m.Ingredients[0].Format != "application/pdf" || m.Ingredients[1].Title != "memo.docx" || m.Ingredients[4].Format != "" {
		t.Errorf("ingredients = %+v", m.Ingredients)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"c2pa.created"`, `"when":"2026-09-06T12:00:00Z"`, `"digitalSourceType":"http://cv.iptc.org/newscodes/digitalsourcetype/trainedAlgorithmicMedia"`, `"softwareAgent":{"name":"draft","version":"0.0.34"}`, `"relationship":"inputTo"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("manifest JSON lacks %s:\n%s", want, b)
		}
	}
	// Without an attribution the counts are zero and the time is now.
	bare := NewManifest(ManifestInput{Body: "x"})
	if bare.Assertions[1].Data.(Grounding).Sentences != 0 || bare.Ingredients != nil {
		t.Errorf("bare manifest = %+v", bare)
	}
}
