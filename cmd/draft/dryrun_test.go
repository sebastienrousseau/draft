// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/pipeline"
)

func dryRunConfig(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	return config.Config{HomeDir: dir, DraftsDir: dir, WriteRetries: 2}
}

// A dry run makes no model calls, so it must work with an engine that would
// fail if called.
func TestDryRunPlansWithoutCallingAModel(t *testing.T) {
	cfg := dryRunConfig(t)
	src := filepath.Join(t.TempDir(), "paper.txt")
	if err := os.WriteFile(src, []byte("The system reached a score of 0.82 on the test set."), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := pipeline.NewRunner(cfg, []engine.Engine{stubEngine{}}, nil)
	var out, errb bytes.Buffer
	if failures := runDryRun(context.Background(), cfg, runner, []pipeline.Job{{Sources: []string{src}}}, &out, &errb); failures != 0 {
		t.Fatalf("expected a clean plan, got %d failure(s): %s", failures, errb.String())
	}

	plan := out.String()
	for _, want := range []string{"Plan", "Sources", "Sections", "Engines", "Model calls", "Output"} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan is missing %q:\n%s", want, plan)
		}
	}
	if !strings.Contains(plan, "paper.txt") {
		t.Errorf("plan does not name the source:\n%s", plan)
	}
}

// An unreadable source is exactly what a dry run exists to surface, before
// committing to a ten-minute run.
func TestDryRunReportsAnUnreadableSource(t *testing.T) {
	cfg := dryRunConfig(t)
	bad := filepath.Join(t.TempDir(), "notes.xyz")
	if err := os.WriteFile(bad, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := pipeline.NewRunner(cfg, []engine.Engine{stubEngine{}}, nil)
	var out, errb bytes.Buffer
	if failures := runDryRun(context.Background(), cfg, runner, []pipeline.Job{{Sources: []string{bad}}}, &out, &errb); failures != 1 {
		t.Errorf("failures = %d, want 1", failures)
	}
	if !strings.Contains(errb.String(), "no readable text") {
		t.Errorf("stderr = %q", errb.String())
	}
}

// The routing is part of the plan: the whole point is seeing which backend
// each stage will use before spending anything.
func TestDryRunShowsPerKindRouting(t *testing.T) {
	cfg := dryRunConfig(t)
	cfg.Engine = config.EngineOllama
	cfg.WriteEngine = "claude"

	src := filepath.Join(t.TempDir(), "paper.txt")
	if err := os.WriteFile(src, []byte("A sentence with a fact in it."), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := pipeline.NewRoutedRunner(cfg, nil)
	var out, errb bytes.Buffer
	runDryRun(context.Background(), cfg, runner, []pipeline.Job{{Sources: []string{src}}}, &out, &errb)

	plan := out.String()
	if !strings.Contains(plan, "extract: ollama") || !strings.Contains(plan, "write: claude") {
		t.Errorf("plan does not show the routing:\n%s", plan)
	}
}

func TestRunDryRunViaFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	src := filepath.Join(dir, "paper.txt")
	if err := os.WriteFile(src, []byte("A sentence with a fact in it."), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	if code := run([]string{"--dry-run", "--engine", "ollama", src}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "Plan") {
		t.Errorf("stdout = %q", out.String())
	}
}
