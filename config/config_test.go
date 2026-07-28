// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package config

import "testing"

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
