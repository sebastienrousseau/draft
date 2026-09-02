// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package config

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// withHome pins the home directory for one test, restoring the real resolver
// afterwards so the tests stay independent of the machine they run on.
func withHome(t *testing.T, home string) {
	t.Helper()
	orig := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = orig })
	t.Setenv("DRAFT_DRAFTS_DIR", "")
	t.Setenv("DRAFT_SOURCES_DIR", "")
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DRAFT_ENGINE", "")
	t.Setenv("DRAFT_MODEL", "")
	c := Load(Flags{})
	if c.Engine != EngineAuto {
		t.Errorf("engine = %q, want auto", c.Engine)
	}
	if c.OllamaModel != DefaultOllamaModel {
		t.Errorf("ollama model = %q", c.OllamaModel)
	}
	if c.ContextLength != DefaultContextLen || c.PredictLength != DefaultPredictLen {
		t.Errorf("ctx/predict = %d/%d", c.ContextLength, c.PredictLength)
	}
	if c.SourcesDir == "" || c.DraftsDir == "" {
		t.Error("paths should be populated")
	}
}

func TestFlagsBeatEnv(t *testing.T) {
	t.Setenv("DRAFT_ENGINE", "ollama")
	t.Setenv("DRAFT_NUM_CTX", "4096")
	c := Load(Flags{Engine: "codex", ContextLength: 2048, Model: "opus", Merge: true, KeepArtifacts: true})
	if c.Engine != "codex" {
		t.Errorf("flag engine should win, got %q", c.Engine)
	}
	if c.ContextLength != 2048 {
		t.Errorf("flag ctx should win, got %d", c.ContextLength)
	}
	if c.Model != "opus" || !c.Merge || !c.KeepArtifacts {
		t.Errorf("flags not applied: %+v", c)
	}
}

func TestRemainingFlagsBeatEnvironment(t *testing.T) {
	t.Setenv("DRAFT_EXTRACT_ENGINE", "env-extract")
	t.Setenv("DRAFT_WRITE_ENGINE", "env-write")
	t.Setenv("DRAFT_NUM_PREDICT", "2048")
	c := Load(Flags{ExtractEngine: "flag-extract", WriteEngine: "flag-write", PredictLength: 4096})
	if c.ExtractEngine != "flag-extract" || c.WriteEngine != "flag-write" || c.PredictLength != 4096 {
		t.Fatalf("remaining flag precedence failed: %+v", c)
	}
}

func TestEnvBeatsDefault(t *testing.T) {
	t.Setenv("DRAFT_ENGINE", "grok")
	t.Setenv("DRAFT_MODEL", "custom-ollama")
	t.Setenv("DRAFT_CLAUDE_MODEL", "opus")
	c := Load(Flags{})
	if c.Engine != "grok" {
		t.Errorf("env engine = %q", c.Engine)
	}
	if c.OllamaModel != "custom-ollama" {
		t.Errorf("DRAFT_MODEL should set ollama model, got %q", c.OllamaModel)
	}
	if c.Model != "opus" {
		t.Errorf("DRAFT_CLAUDE_MODEL alias should set session model, got %q", c.Model)
	}
}

func TestEnvIntGuards(t *testing.T) {
	t.Setenv("DRAFT_NUM_CTX", "not-a-number")
	if c := Load(Flags{}); c.ContextLength != DefaultContextLen {
		t.Errorf("bad int should fall back to default, got %d", c.ContextLength)
	}
	t.Setenv("DRAFT_NUM_CTX", "10") // below the minimum
	if c := Load(Flags{}); c.ContextLength != DefaultContextLen {
		t.Errorf("sub-minimum int should fall back, got %d", c.ContextLength)
	}
}

func TestSessionModelPrecedence(t *testing.T) {
	t.Setenv("DRAFT_MODEL_SESSION", "sonnet")
	t.Setenv("DRAFT_CLAUDE_MODEL", "opus")
	if c := Load(Flags{}); c.Model != "sonnet" {
		t.Errorf("DRAFT_MODEL_SESSION should win over the alias, got %q", c.Model)
	}
}

// draft hardcoded both trees under the home directory with no flag and no
// variable, which made it unusable from CI, from a container, or as a library
// whose caller chooses where output goes.
func TestDirectoriesAreConfigurable(t *testing.T) {
	t.Run("defaults under home", func(t *testing.T) {
		home := t.TempDir()
		withHome(t, home)
		c := Load(Flags{})
		if want := filepath.Join(home, "Drop", "Drafts"); c.DraftsDir != want {
			t.Errorf("DraftsDir = %q, want %q", c.DraftsDir, want)
		}
		if want := filepath.Join(home, "Drop", "Drafts", "Sources"); c.SourcesDir != want {
			t.Errorf("SourcesDir = %q, want %q", c.SourcesDir, want)
		}
	})

	t.Run("environment overrides", func(t *testing.T) {
		home := t.TempDir()
		withHome(t, home)
		t.Setenv("DRAFT_DRAFTS_DIR", "/tmp/out")
		t.Setenv("DRAFT_SOURCES_DIR", "/tmp/src")
		c := Load(Flags{})
		if c.DraftsDir != "/tmp/out" {
			t.Errorf("DraftsDir = %q, want /tmp/out", c.DraftsDir)
		}
		if c.SourcesDir != "/tmp/src" {
			t.Errorf("SourcesDir = %q, want /tmp/src", c.SourcesDir)
		}
	})

	t.Run("flags beat the environment", func(t *testing.T) {
		home := t.TempDir()
		withHome(t, home)
		t.Setenv("DRAFT_DRAFTS_DIR", "/tmp/env")
		c := Load(Flags{DraftsDir: "/tmp/flag", SourcesDir: "/tmp/flagsrc"})
		if c.DraftsDir != "/tmp/flag" {
			t.Errorf("DraftsDir = %q, want /tmp/flag", c.DraftsDir)
		}
		if c.SourcesDir != "/tmp/flagsrc" {
			t.Errorf("SourcesDir = %q, want /tmp/flagsrc", c.SourcesDir)
		}
	})

	t.Run("tilde expands", func(t *testing.T) {
		home := t.TempDir()
		withHome(t, home)
		c := Load(Flags{DraftsDir: "~/elsewhere", SourcesDir: "~"})
		if want := filepath.Join(home, "elsewhere"); c.DraftsDir != want {
			t.Errorf("DraftsDir = %q, want %q", c.DraftsDir, want)
		}
		if c.SourcesDir != home {
			t.Errorf("SourcesDir = %q, want %q", c.SourcesDir, home)
		}
	})

	// A relative Drafts directory writes into whatever directory the process
	// happened to start in — the silent-misplacement bug resolveHome exists to
	// prevent.
	t.Run("relative becomes absolute", func(t *testing.T) {
		home := t.TempDir()
		withHome(t, home)
		c := Load(Flags{DraftsDir: "drafts"})
		if !filepath.IsAbs(c.DraftsDir) {
			t.Errorf("DraftsDir = %q, want an absolute path", c.DraftsDir)
		}
	})
}

// filepath.Abs fails only when the working directory cannot be read. Keep the
// value rather than dropping it, and say so.
func TestAbsDirReportsAnUnresolvablePath(t *testing.T) {
	orig := absPath
	absPath = func(string) (string, error) { return "", errors.New("no cwd") }
	defer func() { absPath = orig }()

	home := t.TempDir()
	withHome(t, home)
	c := Load(Flags{DraftsDir: "relative"})
	if c.DraftsDir != "relative" {
		t.Errorf("DraftsDir = %q, want the unresolved value kept", c.DraftsDir)
	}
	var warned bool
	for _, w := range c.Warnings {
		if strings.Contains(w, "--out") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected a warning naming the setting, got %v", c.Warnings)
	}
}
