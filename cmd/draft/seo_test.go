// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodArticle = `---
title: "Quantum-Safe Readiness for Payment Networks"
description: "What banks must inventory before migrating payment keys to post-quantum algorithms."
author: "Sebastien Rousseau"
url: "https://example.com/q"
schema: "Article"
draft_engine: "claude"
---

## Inventory

Body.
`

func writeArticle(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSEOCleanArticleExitsZero(t *testing.T) {
	p := writeArticle(t, t.TempDir(), "a.md", goodArticle)
	var out, errb bytes.Buffer
	if code := run([]string{"--seo", p}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q, stdout %q", code, errb.String(), out.String())
	}
	if !strings.Contains(out.String(), "Follows Google Search guidance") {
		t.Errorf("stdout %q", out.String())
	}
}

// Two files sharing a title fail as a set even though each passes alone.
func TestSEOSetDuplicateExitsOneAndNamesBoth(t *testing.T) {
	dir := t.TempDir()
	a := writeArticle(t, dir, "a.md", goodArticle)
	b := writeArticle(t, dir, "b.md", strings.Replace(goodArticle, "What banks", "Which records banks", 1))
	var out, errb bytes.Buffer
	if code := run([]string{"--seo", a, b}, &out, &errb); code != 1 {
		t.Fatalf("exit %d, stdout %q", code, out.String())
	}
	if strings.Count(out.String(), "title-duplicate") != 2 {
		t.Errorf("both articles must carry the finding: %q", out.String())
	}
}

func TestSEOJSONOneObjectPerArticle(t *testing.T) {
	dir := t.TempDir()
	a := writeArticle(t, dir, "a.md", goodArticle)
	b := writeArticle(t, dir, "b.md", "No frontmatter at all.\n")
	var out, errb bytes.Buffer
	if code := run([]string{"--json", "--seo", a, b}, &out, &errb); code != 1 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 JSON lines, got %q", out.String())
	}
	var first, second seoReport
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil || first.Findings == nil || len(first.Findings) != 0 {
		t.Errorf("clean article must report an empty findings array: %s (%v)", lines[0], err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil || len(second.Findings) == 0 || second.Path != b {
		t.Errorf("bare body must report findings: %s (%v)", lines[1], err)
	}
}

func TestSEOTextShowsSeverityRuleAndSource(t *testing.T) {
	p := writeArticle(t, t.TempDir(), "a.md", strings.Replace(goodArticle, `schema: "Article"`, `schema: "FAQPage, Article"`, 1))
	var out, errb bytes.Buffer
	if code := run([]string{"--seo", p}, &out, &errb); code != 0 {
		t.Fatalf("a warning alone must not fail: exit %d", code)
	}
	for _, want := range []string{"warning", "structured-data-faq-retired", "developers.google.com/search/docs/appearance/structured-data/faqpage"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout lacks %q: %q", want, out.String())
		}
	}
}

func TestSEOUnreadableFiles(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.md")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxSEOFile + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	for _, p := range []string{filepath.Join(dir, "missing.md"), big} {
		var out, errb bytes.Buffer
		if code := run([]string{"--seo", p}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "draft:") {
			t.Errorf("%s: exit %d, stderr %q", p, code, errb.String())
		}
	}
}
