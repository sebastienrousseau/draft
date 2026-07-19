// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package config resolves runtime configuration from command-line flags,
// environment variables, and sensible defaults, in that order of precedence.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Engine selection sentinels. Any other value names a specific session provider
// (claude, codex, gemini, copilot, cursor-agent, amp, crush, goose, grok, ...).
const (
	// EngineAuto picks the first installed session provider and falls back to
	// Ollama if a call fails (offline). This is the default.
	EngineAuto = "auto"
	// EngineOllama forces the local Ollama backend and never touches a session
	// CLI or the network.
	EngineOllama = "ollama"
)

// Defaults captured as constants so they are documented in one place.
const (
	DefaultOllamaModel  = "qwen3:4b"
	DefaultExtractModel = "gemma3:4b"
	DefaultEditModel    = "gemma3:4b"
	DefaultContextLen   = 8192
	DefaultPredictLen   = 6000
	DefaultWriteRetries = 2
	DefaultMaxContinue  = 3
	OllamaHost          = "http://127.0.0.1:11434"
	FocusBlock          = 25 * time.Minute
)

// Config is the fully-resolved run configuration shared across packages.
type Config struct {
	Engine       string // "auto", "ollama", or a session provider name
	Model        string // session-provider model override ("" = provider default)
	OllamaModel  string // writing model for the Ollama backend
	ExtractModel string // claim-extraction model for the Ollama backend
	EditModel    string // surgical-review model for the Ollama backend

	ContextLength int
	PredictLength int
	WriteRetries  int
	MaxContinue   int // max length-driven continuations for a single generation
	ForceNew      bool
	Merge         bool // combine every input into one draft instead of queueing
	KeepArtifacts bool // keep prompt/ledger files beside a successful draft
	Experimental  bool // let auto-selection consider experimental providers
	OllamaHost    string

	HomeDir    string
	SourcesDir string
	DraftsDir  string
}

// Load builds a Config from defaults, overlays environment variables, then
// overlays the already-parsed flag values passed in from the caller.
func Load(flags Flags) Config {
	home, _ := os.UserHomeDir()
	c := Config{
		Engine:        env("DRAFT_ENGINE", EngineAuto),
		Model:         env("DRAFT_MODEL_SESSION", env("DRAFT_CLAUDE_MODEL", "")),
		OllamaModel:   env("DRAFT_WRITE_MODEL", env("DRAFT_MODEL", DefaultOllamaModel)),
		ExtractModel:  env("DRAFT_EXTRACT_MODEL", env("DRAFT_MODEL", DefaultExtractModel)),
		EditModel:     env("DRAFT_EDIT_MODEL", env("DRAFT_MODEL", DefaultEditModel)),
		ContextLength: envInt("DRAFT_NUM_CTX", DefaultContextLen, 512),
		PredictLength: envInt("DRAFT_NUM_PREDICT", DefaultPredictLen, 1024),
		WriteRetries:  envInt("DRAFT_WRITE_RETRIES", DefaultWriteRetries, 0),
		MaxContinue:   envInt("DRAFT_MAX_CONTINUE", DefaultMaxContinue, 0),
		OllamaHost:    env("OLLAMA_HOST", OllamaHost),
		HomeDir:       home,
		SourcesDir:    filepath.Join(home, "Drop", "Drafts", "Sources"),
		DraftsDir:     filepath.Join(home, "Drop", "Drafts"),
	}

	// Flags win over environment.
	if flags.Engine != "" {
		c.Engine = flags.Engine
	}
	if flags.Model != "" {
		c.Model = flags.Model
	}
	if flags.ContextLength > 0 {
		c.ContextLength = flags.ContextLength
	}
	if flags.PredictLength > 0 {
		c.PredictLength = flags.PredictLength
	}
	c.ForceNew = flags.ForceNew
	c.Merge = flags.Merge
	c.KeepArtifacts = flags.KeepArtifacts
	c.Experimental = flags.Experimental || strings.EqualFold(os.Getenv("DRAFT_EXPERIMENTAL"), "1") || strings.EqualFold(os.Getenv("DRAFT_EXPERIMENTAL"), "true")
	return c
}

// Flags holds the raw command-line values before they are merged into a Config.
type Flags struct {
	Engine        string
	Model         string
	ContextLength int
	PredictLength int
	ForceNew      bool
	Merge         bool
	KeepArtifacts bool
	Experimental  bool
}

func env(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func envInt(name string, fallback, minValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < minValue {
		return fallback
	}
	return v
}
