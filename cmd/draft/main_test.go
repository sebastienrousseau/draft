// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/internal/config"
	"github.com/sebastienrousseau/draft/internal/engine"
	"github.com/sebastienrousseau/draft/internal/pipeline"
)

func tmpSource(t *testing.T, name string) (config.Config, string) {
	t.Helper()
	home := t.TempDir()
	sources := filepath.Join(home, "Drop", "Drafts", "Sources")
	if err := os.MkdirAll(sources, 0o755); err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(sources, name)
	if err := os.WriteFile(full, []byte("Some research. It scored 0.5 on the set."), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{HomeDir: home, SourcesDir: sources, DraftsDir: filepath.Join(home, "Drop", "Drafts"), MaxContinue: 1}
	return cfg, full
}

func TestResolveSourceBareName(t *testing.T) {
	cfg, _ := tmpSource(t, "paper.txt")
	got, err := resolveSource(cfg, "paper.txt")
	if err != nil || !strings.HasSuffix(got, "paper.txt") {
		t.Errorf("bare name resolution failed: %q %v", got, err)
	}
}

func TestResolveSourceAbsoluteAndTilde(t *testing.T) {
	cfg, full := tmpSource(t, "abs.pdf")
	if _, err := resolveSource(cfg, full); err != nil {
		t.Errorf("absolute path should resolve: %v", err)
	}
	rel, _ := filepath.Rel(cfg.HomeDir, full)
	if _, err := resolveSource(cfg, "~/"+rel); err != nil {
		t.Errorf("~ path should resolve: %v", err)
	}
}

func TestResolveSourceMissing(t *testing.T) {
	cfg, _ := tmpSource(t, "x.pdf")
	if _, err := resolveSource(cfg, "nope.pdf"); err == nil {
		t.Error("missing file should error")
	}
}

func TestBuildJobsPerFileAndMerge(t *testing.T) {
	cfg, _ := tmpSource(t, "a.pdf")
	// add a second file
	os.WriteFile(filepath.Join(cfg.SourcesDir, "b.pdf"), []byte("x"), 0o644)

	jobs, err := buildJobs(cfg, []string{"a.pdf", "b.pdf"}, "")
	if err != nil || len(jobs) != 2 {
		t.Fatalf("per-file jobs wrong: %d %v", len(jobs), err)
	}
	cfg.Merge = true
	jobs, err = buildJobs(cfg, []string{"a.pdf", "b.pdf"}, "")
	if err != nil || len(jobs) != 1 || len(jobs[0].Sources) != 2 {
		t.Fatalf("merged job wrong: %+v %v", jobs, err)
	}
}

func TestBuildJobsError(t *testing.T) {
	cfg, _ := tmpSource(t, "a.pdf")
	if _, err := buildJobs(cfg, []string{"missing.pdf"}, ""); err == nil {
		t.Error("expected error for missing source")
	}
}

// stubEngine returns a fixed valid article so runHeadless completes.
type stubEngine struct{}

func (stubEngine) Name() string { return "stub" }
func (stubEngine) Generate(_ context.Context, req engine.Request) (engine.Result, error) {
	if req.Kind == engine.KindExtract {
		return engine.Result{Text: "CLAIM: It scored 0.5 on the set\nSOURCE_QUOTE: \"scored 0.5 on the set\"\nTYPE: metric\nSTRENGTH: demonstrated\n---"}, nil
	}
	body := strings.Repeat("A clear grounded sentence that stands on its own. ", 120)
	return engine.Result{Text: "# Title\n\n**Thesis.**\n\n<aside class=\"post-lead\"></aside>\n\n> **Executive Summary**\n>\n> - point\n\n## Section\n\n" + body + "."}, nil
}

func TestRunHeadless(t *testing.T) {
	cfg, full := tmpSource(t, "paper.txt")
	jobs := []pipeline.Job{{Sources: []string{full}}}
	var out strings.Builder
	failures := runHeadless(context.Background(), cfg, []engine.Engine{stubEngine{}}, jobs, &out, io.Discard)
	if failures != 0 {
		t.Errorf("expected success, got %d failures", failures)
	}
	// The dated output folder should now contain exactly the article.
	dated, _ := filepath.Glob(filepath.Join(cfg.DraftsDir, "*", "*.md"))
	if len(dated) == 0 {
		t.Error("no article written")
	}
	if !strings.Contains(out.String(), ".md") {
		t.Error("stdout should carry the output path")
	}
}

func TestRunHeadlessFailure(t *testing.T) {
	cfg, full := tmpSource(t, "paper.txt")
	jobs := []pipeline.Job{{Sources: []string{full}}}
	// An ollama-only chain with no server -> failure.
	chain := engine.Chain(config.Config{Engine: config.EngineOllama, OllamaHost: "http://127.0.0.1:0"})
	if runHeadless(context.Background(), cfg, chain, jobs, io.Discard, io.Discard) == 0 {
		t.Error("expected a failure with no reachable engine")
	}
}

func TestUsageMentionsProviders(t *testing.T) {
	var buf strings.Builder
	usage(&buf)
	for _, want := range []string{"USAGE", "claude", "codex", "--engine", "--keep-artifacts"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("usage missing %q", want)
		}
	}
}

func TestRunVersion(t *testing.T) {
	var out, errb strings.Builder
	if code := run([]string{"--version"}, &out, &errb); code != 0 {
		t.Errorf("version exit = %d", code)
	}
	if !strings.Contains(out.String(), version) {
		t.Errorf("version not printed: %q", out.String())
	}
}

func TestRunNoArgs(t *testing.T) {
	var out, errb strings.Builder
	if code := run(nil, &out, &errb); code != 2 {
		t.Errorf("no-args exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "USAGE") {
		t.Error("usage should print to stderr")
	}
}

func TestRunBadFlag(t *testing.T) {
	var out, errb strings.Builder
	if code := run([]string{"--nope"}, &out, &errb); code != 2 {
		t.Errorf("bad flag exit = %d, want 2", code)
	}
}

func TestRunMissingSource(t *testing.T) {
	var out, errb strings.Builder
	if code := run([]string{"--engine", "ollama", "/no/such/file.pdf"}, &out, &errb); code != 1 {
		t.Errorf("missing source exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "not found") {
		t.Error("should report the missing source")
	}
}

func TestRunHeadlessViaRun(t *testing.T) {
	_, full := tmpSource(t, "paper.txt")
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:0")
	var out, errb strings.Builder
	// --print with a forced, unreachable ollama exits non-zero after failing.
	if code := run([]string{"--print", "--engine", "ollama", full}, &out, &errb); code != 1 {
		t.Errorf("headless failure exit = %d, want 1", code)
	}
}

func TestResolveSourceTildeRoot(t *testing.T) {
	cfg, _ := tmpSource(t, "x.txt")
	if _, err := resolveSource(cfg, "~"); err != nil {
		t.Errorf("~ should resolve to home: %v", err)
	}
}

func TestBuildJobsReview(t *testing.T) {
	cfg, _ := tmpSource(t, "paper.txt")
	draft := filepath.Join(t.TempDir(), "existing.md")
	if err := os.WriteFile(draft, []byte("# X\n\n## S\n\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	jobs, err := buildJobs(cfg, []string{"paper.txt"}, draft)
	if err != nil || len(jobs) != 1 || jobs[0].ReviewPath == "" {
		t.Fatalf("review job build failed: %v %+v", err, jobs)
	}
	if _, err := buildJobs(cfg, nil, draft); err == nil {
		t.Error("--review with no sources should error")
	}
	if _, err := buildJobs(cfg, []string{"paper.txt"}, "/no/such.md"); err == nil {
		t.Error("--review with a missing draft should error")
	}
}
