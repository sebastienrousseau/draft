// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"strings"
	"testing"
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
	}
	for want, in := range checks {
		if !hasSubstr(Errors(in), want) {
			t.Errorf("expected error %q for its input", want)
		}
	}
}
