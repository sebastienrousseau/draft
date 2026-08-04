// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/brand"
	"github.com/sebastienrousseau/draft/pipeline"
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
	failures := runHeadless(context.Background(), cfg, pipeline.NewRunner(cfg, []engine.Engine{stubEngine{}}, nil), jobs, &out, io.Discard)
	if failures != 0 {
		t.Errorf("expected success, got %d failures", failures)
	}
	// The dated output folder holds the article as a day-folder set.
	finals, _ := filepath.Glob(filepath.Join(cfg.DraftsDir, "*", "final", "*-final.md"))
	if len(finals) == 0 {
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
	if runHeadless(context.Background(), cfg, pipeline.NewRunner(cfg, chain, nil), jobs, io.Discard, io.Discard) == 0 {
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

func TestUsageIsBranded(t *testing.T) {
	var b strings.Builder
	usage(&b)
	out := b.String()

	// The command's own surface carries the same identity as the dashboard.
	for _, want := range []string{"⣰⣿", brand.Wordmark, "From paper to post. Grounded."} {
		if !strings.Contains(out, want) {
			t.Errorf("usage is missing branding %q", want)
		}
	}
	// And it documents every flag the binary actually accepts.
	for _, want := range []string{"--frontmatter", "--combine", "DRAFT_SITE_"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage does not document %q", want)
		}
	}
}

func TestUsageLogoDisabledByEnv(t *testing.T) {
	t.Setenv("DRAFT_SHOW_LOGO", "0")
	var b strings.Builder
	usage(&b)
	if out := b.String(); strings.Contains(out, "⣰⣿") {
		t.Error("DRAFT_SHOW_LOGO=0 should suppress the logo in usage")
	} else if !strings.Contains(out, "USAGE") {
		t.Error("usage text must still render without the logo")
	}
}

func TestRunFrontmatterFlagConflicts(t *testing.T) {
	dir := t.TempDir()
	draftPath := filepath.Join(dir, "2026-07-27-conflict.md")
	if err := os.WriteFile(draftPath, []byte("# T\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errb strings.Builder
	if code := run([]string{"--frontmatter", draftPath, "--review", draftPath}, &out, &errb); code != 2 {
		t.Errorf("--frontmatter with --review should exit 2, got %d", code)
	}
	errb.Reset()
	if code := run([]string{"--frontmatter", draftPath, "source.txt"}, &out, &errb); code != 2 {
		t.Errorf("--frontmatter with positional args should exit 2, got %d", code)
	}
}

func TestRunFrontmatterFlag(t *testing.T) {
	dir := t.TempDir()
	draftPath := filepath.Join(dir, "2026-07-26-test-article.md")
	content := "# Test Article\n\n**Test Subtitle**\n\nProse content for test.\n"
	if err := os.WriteFile(draftPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errb strings.Builder
	code := run([]string{"--frontmatter", draftPath}, &out, &errb)
	if code != 0 {
		t.Fatalf("frontmatter flag failed with code %d: %s", code, errb.String())
	}

	if !strings.Contains(out.String(), "Frontmatter regenerated successfully") {
		t.Errorf("unexpected output: %s", out.String())
	}

	bodyFile := filepath.Join(dir, "2026-07-26-test-article-body.md")
	fmFile := filepath.Join(dir, "2026-07-26-test-article-frontmatter.yaml")
	finalFile := filepath.Join(dir, "2026-07-26-test-article-final.md")

	if _, err := os.Stat(bodyFile); err != nil {
		t.Errorf("body file missing: %v", err)
	}
	if _, err := os.Stat(fmFile); err != nil {
		t.Errorf("frontmatter file missing: %v", err)
	}
	if _, err := os.Stat(finalFile); err != nil {
		t.Errorf("final file missing: %v", err)
	}
}

func TestVersionDerivedFromBuildInfo(t *testing.T) {
	// The version must not be a hand-maintained literal: it is released from a
	// tag, and a second source of truth is what let v0.0.22 ship mislabelled.
	if version == "" {
		t.Fatal("version is empty")
	}
	// A go-install build reports the module version; a plain `go build` or
	// `go test` has none, and must fall back to a readable placeholder rather
	// than a stale hardcoded number.
	if version != "dev" && !strings.HasPrefix(version, "v") && !strings.HasPrefix(version, "0.") {
		t.Errorf("unexpected version form %q", version)
	}
}

func TestRunHeadlessJSON(t *testing.T) {
	cfg, full := tmpSource(t, "paper.txt")
	jobs := []pipeline.Job{{Sources: []string{full}}}
	var out strings.Builder
	if failures := runHeadlessJSON(context.Background(), cfg, pipeline.NewRunner(cfg, []engine.Engine{stubEngine{}}, nil), jobs, &out, io.Discard); failures != 0 {
		t.Fatalf("expected success, got %d failures", failures)
	}
	// One JSON object per line, machine-readable.
	var rec struct {
		Source, Output, Engine, Mode string
		Words                        int
		OK                           bool
	}
	line := strings.TrimSpace(out.String())
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, line)
	}
	if !rec.OK || rec.Words == 0 || rec.Engine == "" || !strings.HasSuffix(rec.Output, "-final.md") {
		t.Errorf("unexpected record: %+v", rec)
	}
}

func TestPrintPlanReportsResumableLedger(t *testing.T) {
	var out strings.Builder
	printPlan(&out, config.Config{}, pipeline.DryRunReport{
		Sources:     []string{"paper.md"},
		LedgerFound: true,
		EstCalls:    1,
		Engines:     map[engine.Kind]string{},
	})
	if !strings.Contains(out.String(), "extraction resumable") {
		t.Fatalf("resumable ledger was not reported: %s", out.String())
	}
}

func TestRunDryRunLabelsMultipleJobs(t *testing.T) {
	cfg, first := tmpSource(t, "first.txt")
	second := filepath.Join(cfg.SourcesDir, "second.txt")
	if err := os.WriteFile(second, []byte("More research with a score of 0.7."), 0o644); err != nil {
		t.Fatal(err)
	}
	jobs := []pipeline.Job{{Sources: []string{first}}, {Sources: []string{second}}}
	var out strings.Builder
	r := pipeline.NewRunner(cfg, []engine.Engine{stubEngine{}}, nil)
	if failures := runDryRun(context.Background(), cfg, r, jobs, &out, io.Discard); failures != 0 {
		t.Fatalf("runDryRun returned %d failures", failures)
	}
	if !strings.Contains(out.String(), "[1/2]") || !strings.Contains(out.String(), "[2/2]") {
		t.Fatalf("multi-job labels missing: %s", out.String())
	}
}

func TestCompletionScripts(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		var out strings.Builder
		if code := run([]string{"--completion", shell}, &out, io.Discard); code != 0 {
			t.Errorf("--completion %s exited %d", shell, code)
		}
		// fish spells long options as "-l frontmatter", bash/zsh as "--frontmatter".
		if !strings.Contains(out.String(), "frontmatter") || !strings.Contains(out.String(), "draft") {
			t.Errorf("%s completion does not mention the flags", shell)
		}
	}
	var errb strings.Builder
	if code := run([]string{"--completion", "nonsense"}, io.Discard, &errb); code != 2 {
		t.Error("an unknown shell should exit 2")
	}
}

func TestBuildVersionFallback(t *testing.T) {
	// Under `go test` the module has no released version, so the fallback
	// must be a readable placeholder rather than an empty string.
	if got := buildVersion(); got == "" {
		t.Error("buildVersion must never return an empty string")
	}
}

func TestBuildVersionFromModuleMetadata(t *testing.T) {
	orig := readBuildInfo
	defer func() { readBuildInfo = orig }()
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, true
	}
	if got := buildVersion(); got != "1.2.3" {
		t.Fatalf("build version = %q", got)
	}
}

func TestRunTUIDispatch(t *testing.T) {
	orig := runTeaProgram
	defer func() { runTeaProgram = orig }()
	_, full := tmpSource(t, "paper.txt")

	calls := 0
	runTeaProgram = func(tea.Model, ...tea.ProgramOption) (tea.Model, error) {
		calls++
		return nil, nil
	}
	if code := run([]string{full}, io.Discard, io.Discard); code != 0 || calls != 1 {
		t.Fatalf("successful TUI dispatch = code %d, calls %d", code, calls)
	}

	runTeaProgram = func(tea.Model, ...tea.ProgramOption) (tea.Model, error) {
		return nil, errors.New("terminal failed")
	}
	var stderr strings.Builder
	if code := run([]string{"--engine", "ollama", full}, io.Discard, &stderr); code != 1 || !strings.Contains(stderr.String(), "terminal failed") {
		t.Fatalf("failed TUI dispatch = code %d, stderr %q", code, stderr.String())
	}
}

func TestRunDryRunSuccessAndFailure(t *testing.T) {
	_, source := tmpSource(t, "paper.txt")
	var out, stderr strings.Builder
	if code := run([]string{"--dry-run", "--engine", "ollama", source}, &out, &stderr); code != 0 || !strings.Contains(out.String(), "Plan") {
		t.Fatalf("dry run = code %d, out %q, err %q", code, out.String(), stderr.String())
	}

	bad := filepath.Join(t.TempDir(), "source.bin")
	if err := os.WriteFile(bad, []byte("unsupported"), 0o644); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := run([]string{"--dry-run", "--engine", "ollama", bad}, io.Discard, &stderr); code != 1 {
		t.Fatalf("invalid dry run = code %d, err %q", code, stderr.String())
	}
}

func TestRunFrontmatterMissingFile(t *testing.T) {
	var stderr strings.Builder
	if code := run([]string{"--frontmatter", filepath.Join(t.TempDir(), "missing.md")}, io.Discard, &stderr); code != 1 {
		t.Fatalf("missing frontmatter file = code %d, err %q", code, stderr.String())
	}
}

func TestMaxHelper(t *testing.T) {
	if max(3, 7) != 7 || max(7, 3) != 7 || max(4, 4) != 4 {
		t.Error("max is wrong")
	}
}

func TestRunHeadlessJSONReportsFailure(t *testing.T) {
	cfg, full := tmpSource(t, "paper.txt")
	jobs := []pipeline.Job{{Sources: []string{full}}}
	// An ollama-only chain with no server: the job must fail, and the JSON
	// record must say so rather than claiming success.
	chain := engine.Chain(config.Config{Engine: config.EngineOllama, OllamaHost: "http://127.0.0.1:0"})
	var out strings.Builder
	if failures := runHeadlessJSON(context.Background(), cfg, pipeline.NewRunner(cfg, chain, nil), jobs, &out, io.Discard); failures == 0 {
		t.Fatal("expected a failure with no reachable engine")
	}
	var rec struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &rec); err != nil {
		t.Fatalf("failure output is not valid JSON: %v", err)
	}
	if rec.OK || rec.Error == "" {
		t.Errorf("a failed job must report ok=false with an error: %+v", rec)
	}
}

func TestRunJSONFlagViaRun(t *testing.T) {
	_, full := tmpSource(t, "paper.txt")
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:0")
	var out, errb strings.Builder
	if code := run([]string{"--json", "--engine", "ollama", full}, &out, &errb); code != 1 {
		t.Errorf("--json failure exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Errorf("--json should emit a record even on failure: %q", out.String())
	}
}
