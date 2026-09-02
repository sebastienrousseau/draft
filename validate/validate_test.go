// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/claims"
)

func TestEndsSentence(t *testing.T) {
	cases := []struct {
		tail string
		want bool
	}{
		{"A complete sentence.", true},
		{`Ends on a straight quote."`, true},
		{"The author called it the “spark.”", true}, // smart close-quote (multibyte)
		{"It trailed off…", true},                   // ellipsis (multibyte)
		{"the team’s results.", true},               // curly apostrophe mid, period end
		{"A ledger block\n\n---", true},
		{"cut off mid-thought and", false},
		{"no terminal punctuation", false},
	}
	for _, c := range cases {
		if got := EndsSentence(c.tail); got != c.want {
			t.Errorf("EndsSentence(%q) = %v, want %v", c.tail, got, c.want)
		}
	}
}

func TestErrorsStructure(t *testing.T) {
	good := "# Title\n\n**Thesis.**\n\n" +
		`<aside class="post-lead" aria-label="Article summary"></aside>` + "\n\n" +
		"> **Executive Summary**\n>\n> - point\n\n## Section\n\n" +
		filler(600) + "."
	if errs := Errors(good); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	bad := "Just some prose with no structure and a leverage of banned words."
	errs := Errors(bad)
	if len(errs) == 0 {
		t.Fatal("expected structural and banned-word errors")
	}
	if !contains(errs, "contains banned words: leverage") {
		t.Errorf("expected banned-word error, got %v", errs)
	}
}

func TestErrorsCatchesInflectedBannedWord(t *testing.T) {
	// An inflected banned word must still be flagged; the base-form-only matcher
	// missed "leverages" and "leveraging".
	base := "# T\n\n**x**\n\n<aside class=\"post-lead\"></aside>\n\nExecutive Summary\n\n## S\n\n"
	for _, w := range []string{"leverages", "leveraging", "utilizes", "fostered"} {
		md := base + filler(600) + " it " + w + " the data."
		if !hasSubstr(Errors(md), "banned words") {
			t.Errorf("expected %q to be flagged as a banned word", w)
		}
	}
}

func TestContainsEmoji(t *testing.T) {
	if !ContainsEmoji("a rocket 🚀 here") {
		t.Error("expected emoji detected")
	}
	if ContainsEmoji("plain ascii — with em dash") {
		t.Error("did not expect emoji")
	}
}

func filler(n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += "word "
	}
	return out
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func TestErrorsEachMissingElement(t *testing.T) {
	base := "# T\n\n<aside class=\"post-lead\"></aside>\n\nExecutive Summary\n\n## S\n\n" + filler(600) + "."
	// Sanity: base is valid.
	if e := Errors(base); len(e) != 0 {
		t.Fatalf("base should be valid: %v", e)
	}
	checks := map[string]string{
		"body-only mode must start": "no h1 " + base[2:],
		"missing post-lead aside":   strings.Replace(base, `<aside class="post-lead">`, "", 1),
		"missing Executive Summary": strings.Replace(base, "Executive Summary", "Overview", 1),
		"missing section headings":  strings.Replace(base, "## S", "S", 1),
		"contains emoji":            base + " 🚀",
		"minimum is":                "# T\n\n<aside class=\"post-lead\"></aside>\n\nExecutive Summary\n\n## S\n\ntiny.",
		"unfilled skeleton":         strings.Replace(base, "## S", "## ...", 1),
	}
	for want, in := range checks {
		if !hasSubstr(Errors(in), want) {
			t.Errorf("expected error %q for its input", want)
		}
	}
}

// enforceStyle refuses to rewrite a quotation, so failing the draft for a
// banned word inside one would be a rule the repair pass can never satisfy —
// an unfixable rewrite loop.
func TestErrorsIgnoresBannedWordsInsideQuotations(t *testing.T) {
	base := "# Title\n\n<aside class=\"post-lead\"></aside>\n\nExecutive Summary\n\n## S\n\n" +
		strings.Repeat("word ", 600) + "."
	quoted := base + "\n\nThe paper states: \"we leverage it\".\n"
	for _, e := range Errors(quoted) {
		if strings.Contains(e, "banned words") {
			t.Errorf("Errors() flagged a banned word inside a quotation: %q", e)
		}
	}

	// The same word in the writer's own prose must still fail.
	unquoted := base + "\n\nWe leverage it.\n"
	var found bool
	for _, e := range Errors(unquoted) {
		if strings.Contains(e, "banned words") {
			found = true
		}
	}
	if !found {
		t.Errorf("Errors() missed a banned word in unquoted prose: %v", Errors(unquoted))
	}
}

// A number in the article that appears in no claim is the clearest sign of
// invention. Warning about it and saving anyway is hard to defend for a tool
// whose promise is that every sentence is grounded.
func TestStrictNumbersPromotesInventionToAnError(t *testing.T) {
	recs := []claims.Record{{Claim: "throughput reached 12 pages/s", SourceQuote: "throughput reached 12 pages/s"}}
	article := "The model improved by 34%, reaching 12 pages/s."

	errs, warnings := Faithfulness(article, recs)
	if hasNumberComplaint(errs) {
		t.Error("default policy should not block on an ungrounded number")
	}
	if !hasNumberComplaint(warnings) {
		t.Errorf("default policy should warn, got %v", warnings)
	}

	errs, warnings = FaithfulnessWithOptions(article, recs, Options{StrictNumbers: true})
	if !hasNumberComplaint(errs) {
		t.Errorf("strict policy should block, got errs %v", errs)
	}
	if hasNumberComplaint(warnings) {
		t.Error("strict policy should not also warn")
	}
}

// The two dominant false positives must not fail an honest draft.
func TestStrictNumbersIgnoresStructureAndYears(t *testing.T) {
	recs := []claims.Record{{Claim: "a claim", SourceQuote: "a claim"}}
	for _, tc := range []struct{ name, article string }{
		{"ordered list", "1. first item\n2. second item\n3. third item\n"},
		{"publication year", "The approach was described in 2019 and refined later.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs, _ := FaithfulnessWithOptions(tc.article, recs, Options{StrictNumbers: true})
			if hasNumberComplaint(errs) {
				t.Errorf("strict policy blocked on %s: %v", tc.name, errs)
			}
		})
	}
	// Both are still reported when the result is only advisory, so nothing is
	// hidden from a reader of the warnings.
	_, warnings := Faithfulness("The approach was described in 2019.", recs)
	if !hasNumberComplaint(warnings) {
		t.Error("advisory mode should still report a year")
	}
}

func hasNumberComplaint(msgs []string) bool {
	for _, m := range msgs {
		if strings.Contains(m, "numbers not found in any claim") {
			return true
		}
	}
	return false
}
