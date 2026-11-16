// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package seo

import (
	"strings"
	"testing"
)

// clean is an article every rule passes; each case below breaks one thing.
func clean() Article {
	return Article{
		Path: "a.md",
		Fields: map[string]string{
			"title":        "Quantum-Safe Readiness for Payment Networks",
			"description":  "What banks must inventory before migrating payment keys to post-quantum algorithms, and why the deadline is earlier than it looks.",
			"author":       "Sebastien Rousseau",
			"url":          "https://example.com/quantum-safe",
			"permalink":    "https://example.com/quantum-safe/",
			"schema":       "Article",
			"draft_engine": "claude",
			"image":        "https://cdn.example.com/a.webp",
			"image_alt":    "A portrait of the author",
		},
		Body: "## Inventory\n\nSee [the NIST transition report](https://csrc.nist.gov/pubs/ir/8547).\n\n![Key inventory flow](x.png)\n",
	}
}

func rulesOf(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Rule)
	}
	return out
}

func TestCleanArticleHasNoFindings(t *testing.T) {
	if fs := Check(clean()); len(fs) != 0 {
		t.Fatalf("clean article: got %v", fs)
	}
}

func TestEachRuleFires(t *testing.T) {
	cases := []struct {
		rule string
		sev  Severity
		edit func(a *Article)
	}{
		{"title-missing", Error, func(a *Article) { delete(a.Fields, "title") }},
		{"title-boilerplate", Error, func(a *Article) { a.Fields["title"] = "Untitled" }},
		{"title-boilerplate", Error, func(a *Article) { a.Fields["title"] = "Untitled Article" }},
		{"title-boilerplate", Error, func(a *Article) { a.Fields["name"] = "Seb"; a.Fields["title"] = "seb" }},
		{"title-repeated-word", Error, func(a *Article) { a.Fields["title"] = "Payments, payments and PAYMENTS" }},
		{"description-missing", Error, func(a *Article) { a.Fields["description"] = "  " }},
		{"description-too-short", Error, func(a *Article) { a.Fields["description"] = "Quantum payments" }},
		{"description-keyword-list", Error, func(a *Article) { a.Fields["description"] = "PQC, payments, quantum risk, NIST, banks" }},
		{"description-truncated", Warning, func(a *Article) { a.Fields["description"] = "A summary that stops mid-sentence, at the point where..." }},
		{"description-truncated", Warning, func(a *Article) { a.Fields["description"] = "A summary that stops mid-sentence, at the point where…" }},
		{"description-repeats-title", Warning, func(a *Article) { a.Fields["description"] = strings.ToUpper(a.Fields["title"]) }},
		{"image-alt-missing", Error, func(a *Article) { a.Fields["image_alt"] = "" }},
		{"image-alt-missing", Error, func(a *Article) { a.Body += "\n![ ](y.png)\n" }},
		{"image-alt-missing", Error, func(a *Article) { a.Body += "\n<img src=\"y.png\" alt=\"\">\n" }},
		{"link-generic-text", Warning, func(a *Article) { a.Body += "\nRead it [here](https://x.org/p).\n" }},
		{"link-generic-text", Warning, func(a *Article) { a.Body += "\n[https://x.org/p](https://x.org/p)\n" }},
		{"link-to-site-root", Warning, func(a *Article) {
			a.Body += "\n[Challenging Anthropomorphic Bias in Robotics ⧉](https://www.economist.com/)\n"
		}},
		{"heading-multiple-h1", Warning, func(a *Article) { a.Body = "# One\n\n# Two\n" + a.Body }},
		{"byline-missing", Error, func(a *Article) { delete(a.Fields, "author") }},
		{"disclosure-missing", Warning, func(a *Article) { delete(a.Fields, "draft_engine") }},
		{"canonical-missing", Error, func(a *Article) { delete(a.Fields, "url"); delete(a.Fields, "permalink") }},
		{"canonical-conflict", Error, func(a *Article) { a.Fields["permalink"] = "https://example.com/other" }},
		{"structured-data-missing", Warning, func(a *Article) { delete(a.Fields, "schema") }},
		{"structured-data-faq-retired", Warning, func(a *Article) { a.Fields["schema"] = "FAQPage, Article" }},
	}
	for _, c := range cases {
		a := clean()
		c.edit(&a)
		fs := Check(a)
		if len(fs) != 1 || fs[0].Rule != c.rule || fs[0].Severity != c.sev {
			t.Errorf("%s: got %v", c.rule, fs)
			continue
		}
		if !strings.HasPrefix(fs[0].Source, "https://developers.google.com/search/") || fs[0].Message == "" {
			t.Errorf("%s: finding lacks a message or a Google source: %+v", c.rule, fs[0])
		}
	}
}

// Things the guidance says do not matter must never produce a finding.
func TestIgnoredByGoogleIsNotChecked(t *testing.T) {
	a := clean()
	a.Fields["title"] = "What Quantum-Safe Readiness Requires Now: Inventory, Crypto-Agility, Vendor Contracts, Migration Sequencing and Board Accountability Across Global Payment Networks"
	a.Fields["description"] = strings.Repeat("A long but genuine summary sentence. ", 20)
	a.Fields["keywords"] = ""
	a.Body = "### Starts at level three\n\n# Then one title\n\nShort."
	if fs := Check(a); len(fs) != 0 {
		t.Fatalf("length, keywords and heading order are not Google rules: %v", fs)
	}
}

func TestNotFooledByLookalikes(t *testing.T) {
	a := clean()
	a.Body = "```\n# one\n# two\n![](z.png)\n[here](https://x.org)\n```\n" +
		"[Payments outlook report 2026 edition](https://x.org/report)\n" +
		"[A specific work with title](https://x.org/?id=3)\n" +
		"[Relative link to a page](/notes)\n" +
		"<img src=a.png alt='diagram'>\n"
	a.Fields["title"] = "Payments after payments regulation"
	a.Fields["description"] = "PQC, payments, quantum risk. NIST, banks"
	if fs := Check(a); len(fs) != 0 {
		t.Fatalf("got %v", fs)
	}
}

func TestDescriptionWithLongPartsIsNotAKeywordList(t *testing.T) {
	if isKeywordList("payments in the cloud era, quantum risk, NIST, banks") {
		t.Fatal("a part longer than three words is prose, not a keyword")
	}
	if isKeywordList("payments, quantum, banks") {
		t.Fatal("fewer than four parts is not a long string of keywords")
	}
	if !isKeywordList("quantum risk management, payments, banks, NIST") {
		t.Fatal("four short parts are a keyword list")
	}
}

func TestThreeWordDescriptionIsLongEnough(t *testing.T) {
	a := clean()
	a.Fields["description"] = "Quantum payment readiness"
	if fs := Check(a); len(fs) != 0 {
		t.Fatalf("Google's floor is one or two words: %v", fs)
	}
}

func TestCheckSetFlagsDuplicatesOnBoth(t *testing.T) {
	a, b, c := clean(), clean(), clean()
	b.Path, c.Path = "b.md", "c.md"
	c.Fields["title"] = "Something else entirely"
	c.Fields["description"] = "A different summary of a different page about other things."
	delete(b.Fields, "title") // empty values are not duplicates of each other
	got := CheckSet([]Article{a, b, c})
	if r := rulesOf(got[0]); len(r) != 1 || r[0] != "description-duplicate" {
		t.Errorf("a: %v", got[0])
	}
	if r := rulesOf(got[1]); len(r) != 2 || r[0] != "title-missing" || r[1] != "description-duplicate" {
		t.Errorf("b: %v", got[1])
	}
	if len(got[2]) != 0 {
		t.Errorf("c: %v", got[2])
	}
	if !strings.Contains(got[0][0].Message, "b.md") || !strings.Contains(got[1][1].Message, "a.md") {
		t.Errorf("duplicate findings must name the other article: %v / %v", got[0][0], got[1][1])
	}
}

func TestParseUnquotes(t *testing.T) {
	doc := "---\ntitle: \"Say \\\"hi\\\"\"\nauthor: 'O''Brien'\nschema: Article\nbroken: \"a\\qb\"\n---\n\nBody text.\n"
	a := Parse("p.md", doc)
	want := map[string]string{"title": `Say "hi"`, "author": "O'Brien", "schema": "Article", "broken": `a\qb`}
	for k, v := range want {
		if a.Fields[k] != v {
			t.Errorf("%s = %q, want %q", k, a.Fields[k], v)
		}
	}
	if a.Body != "Body text." || a.Path != "p.md" {
		t.Errorf("body %q path %q", a.Body, a.Path)
	}
	if got := Parse("n.md", "Just a body."); len(got.Fields) != 0 || got.Body != "Just a body." {
		t.Errorf("no frontmatter: %+v", got)
	}
}

func TestHasErrors(t *testing.T) {
	if HasErrors(nil) || HasErrors([]Finding{{Severity: Warning}}) {
		t.Error("warnings alone are not errors")
	}
	if !HasErrors([]Finding{{Severity: Warning}, {Severity: Error}}) {
		t.Error("an error must be reported")
	}
}

func TestIsSiteRootRejectsUnparseable(t *testing.T) {
	if isSiteRoot("http://[::1") || isSiteRoot("/") {
		t.Error("an unparseable or host-less target is not a site root")
	}
}

func FuzzParseCheck(f *testing.F) {
	f.Add("---\ntitle: \"x\"\n---\n\n# a\n![](b)\n[c](https://d/)")
	f.Fuzz(func(t *testing.T, doc string) {
		_ = Check(Parse("f.md", doc)) // must not panic on any input
	})
}
