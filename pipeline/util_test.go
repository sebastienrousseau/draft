// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/rules"
	"github.com/sebastienrousseau/draft/validate"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Hello, World!":      "hello-world",
		"  Trim -- Dashes  ": "trim-dashes",
		"":                   "draft-article",
		"!!!":                "draft-article",
		"UPPER Case 123":     "upper-case-123",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlugifyLongTruncates(t *testing.T) {
	long := ""
	for i := 0; i < 200; i++ {
		long += "a"
	}
	if got := slugify(long); len(got) > 90 {
		t.Errorf("slug too long: %d", len(got))
	}
}

func TestExtractTitle(t *testing.T) {
	if got := extractTitle("# My Title\n\nbody"); got != "My Title" {
		t.Errorf("got %q", got)
	}
	if got := extractTitle("no title"); got != "draft-article" {
		t.Errorf("fallback title wrong: %q", got)
	}
}

func TestCleanOutputAndStripThinking(t *testing.T) {
	raw := "\x1b[31mpreamble\x1b[0m\nnoise\n# Real Title\n\nbody\x00"
	cleaned := cleanOutput(raw)
	if wantAbsent := "\x1b["; contains(cleaned, wantAbsent) {
		t.Error("ANSI not stripped")
	}
	md := stripThinking(cleaned)
	if md[:2] != "# " {
		t.Errorf("stripThinking should start at H1, got %q", md[:min(20, len(md))])
	}
}

func TestStripThinkingWithTags(t *testing.T) {
	got := stripThinking("<think>reasoning</think>\n# Title\n\nbody")
	if got[:7] != "# Title" {
		t.Errorf("think block not removed: %q", got)
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	if got := uniquePath(p); got != p {
		t.Errorf("non-existent path should be returned as-is")
	}
	os.WriteFile(p, []byte("x"), 0o644)
	got := uniquePath(p)
	if got == p || filepath.Base(got) != "a-2.md" {
		t.Errorf("expected a-2.md, got %q", got)
	}
}

func TestShortPath(t *testing.T) {
	cfg := config.Config{HomeDir: "/home/seb"}
	if got := shortPath(cfg, "/home/seb/Drop/x.md"); got != "~/Drop/x.md" {
		t.Errorf("got %q", got)
	}
	if got := shortPath(cfg, "/other/x.md"); got != "/other/x.md" {
		t.Errorf("non-home path should be unchanged, got %q", got)
	}
}

func TestEnforceStyleSkipsEmptyReplacement(t *testing.T) {
	original := styleReplacers
	styleReplacers = []styleReplacer{{re: regexp.MustCompile("anything"), with: ""}}
	t.Cleanup(func() { styleReplacers = original })

	if got := enforceStyle("anything"); got != "anything" {
		t.Fatalf("enforceStyle changed text for an empty replacement: %q", got)
	}
}

func TestLoadTemplates(t *testing.T) {
	// No template dir -> empty (built-in style used downstream).
	if got := loadTemplates(config.Config{HomeDir: t.TempDir(), DraftsDir: t.TempDir()}); got != "" {
		t.Errorf("expected empty templates, got %q", got)
	}
	// A populated Drop/Templates dir -> non-empty calibration block.
	home := t.TempDir()
	tdir := filepath.Join(home, "Drop", "Templates")
	os.MkdirAll(tdir, 0o755)
	os.WriteFile(filepath.Join(tdir, "sample.md"), []byte("# Heading\n\nStyle sample body."), 0o644)
	got := loadTemplates(config.Config{HomeDir: home, DraftsDir: filepath.Join(home, "Drop", "Drafts")})
	if got == "" || !contains(got, "Style sample") {
		t.Errorf("expected template block, got %q", got)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// A banned word inside a quotation is the source's word, not the writer's.
// Rewriting it attributes to a paper something the paper never said, which is
// the one edit that would void the grounding guarantee.
func TestEnforceStyleLeavesQuotationsIntact(t *testing.T) {
	for _, tc := range []struct{ name, in string }{
		{"straight quote", `The paper states: "we leverage a robust seamless pipeline" (p. 4).`},
		{"smart quote", "The paper states: “we leverage a robust pipeline” (p. 4)."},
		{"inline code", "The flag is `--leverage-robust` in their tool."},
		{"fenced block", "Their snippet:\n\n```\nleverage(robust)\n```\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := enforceStyle(tc.in); got != tc.in {
				t.Errorf("enforceStyle mutated protected text:\n got %q\nwant %q", got, tc.in)
			}
		})
	}
}

// The repair must still fire on the writer's own prose, including prose that
// sits in the same sentence as a protected span.
func TestEnforceStyleStillRepairsUnquotedProse(t *testing.T) {
	in := `We leverage the result: "they leverage it too".`
	got := enforceStyle(in)
	if strings.Contains(got, "We leverage") {
		t.Errorf("enforceStyle left the writer's own banned word: %q", got)
	}
	if !strings.Contains(got, `"they leverage it too"`) {
		t.Errorf("enforceStyle rewrote the quotation: %q", got)
	}
}

// A near-duplicate paragraph is redundant by definition, so removing it needs
// no model — and a rule violation otherwise costs a full rewrite, the most
// expensive call in a run.
func TestRepairDuplicatesRemovesTheLaterCopy(t *testing.T) {
	para := "The measured throughput reached twelve pages per second across the corpus, " +
		"which is roughly five times the previous figure recorded on the same hardware. "
	// Long enough that removing the duplicate still clears the house minimum,
	// which the repair refuses to breach.
	filler := strings.Repeat("Distinct sentence with entirely separate wording here. ", 90)
	md := "# Title\n\n" + para + "\n\n" + filler + "\n\n" + para
	if validate.WordCount(md)-validate.WordCount(para) < rules.MinWords {
		t.Fatalf("fixture too short: %d words", validate.WordCount(md))
	}

	repaired, removed := repairDuplicates(md)
	if removed != 1 {
		t.Fatalf("removed %d paragraphs, want 1", removed)
	}
	if strings.Count(repaired, "twelve pages per second") != 1 {
		t.Errorf("the duplicate survived:\n%s", repaired)
	}
	if len(validate.DuplicateParagraphIndexes(repaired)) != 0 {
		t.Error("the repaired article still reports duplicates")
	}
}

func TestRepairDuplicatesLeavesACleanArticleAlone(t *testing.T) {
	md := "# Title\n\n" + strings.Repeat("A unique sentence about the first topic. ", 30) +
		"\n\n" + strings.Repeat("A different sentence about a second topic. ", 30)
	repaired, removed := repairDuplicates(md)
	if removed != 0 || repaired != md {
		t.Errorf("repairDuplicates changed a clean article (removed %d)", removed)
	}
}

// Shipping a too-short draft is a different violation, not a repair.
func TestRepairDuplicatesStopsAtTheHouseMinimum(t *testing.T) {
	para := strings.Repeat("The same sentence repeated for length. ", 25)
	md := "# Title\n\n" + para + "\n\n" + para
	if validate.WordCount(md) >= 2*rules.MinWords {
		t.Skip("fixture is long enough that removal stays above the floor")
	}
	repaired, removed := repairDuplicates(md)
	if removed != 0 {
		t.Errorf("removed %d paragraph(s), dropping the article under the %d-word floor", removed, rules.MinWords)
	}
	if repaired != md {
		t.Error("article was modified despite no removal")
	}
}

// The repair must run inside validateWithRetry, so a near-duplicate paragraph
// is settled without paying for another generation.
func TestValidateWithRetryRepairsDuplicatesInsteadOfRegenerating(t *testing.T) {
	para := "The measured throughput reached twelve pages per second across the corpus, " +
		"which is roughly five times the previous figure recorded on the same hardware. "
	filler := strings.Repeat("Distinct sentence with entirely separate wording here. ", 90)
	md := "# Title\n\n<aside class=\"post-lead\">TL;DR</aside>\n\n" +
		"Executive Summary\n\n## A section\n\n" + para + "\n\n" + filler + "\n\n" + para

	if errs, _ := validate.Faithfulness(md, nil); len(errs) == 0 {
		t.Fatal("fixture should start with a duplicate-paragraph violation")
	}

	eng := &countingEngine{name: "fake", out: md}
	events := make(chan Event, 64)
	r := NewRunner(config.Config{WriteRetries: 2}, []engine.Engine{eng}, events)

	out, err := r.validateWithRetry(context.Background(), "base prompt", md, nil)
	if err != nil {
		t.Fatalf("validateWithRetry returned %v", err)
	}
	if eng.calls.Load() != 0 {
		t.Errorf("the repair should have avoided regeneration, but %d call(s) were made", eng.calls.Load())
	}
	if strings.Count(out, "twelve pages per second") != 1 {
		t.Errorf("the duplicate survived:\n%s", out)
	}
	close(events)
	var logged bool
	for e := range events {
		if l, ok := e.(LogEvent); ok && strings.Contains(string(l), "near-duplicate") {
			logged = true
		}
	}
	if !logged {
		t.Error("the repair should be reported")
	}
}
