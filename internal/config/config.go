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

// EngineMode selects which generation backend the pipeline prefers.
type EngineMode string

const (
	// EngineAuto uses Claude when the machine is online and the claude CLI is
	// available, and falls back to Ollama otherwise. This is the default.
	EngineAuto EngineMode = "auto"
	// EngineClaude forces the Claude backend and fails fast when it is
	// unavailable.
	EngineClaude EngineMode = "claude"
	// EngineOllama forces the local Ollama backend and never touches the network.
	EngineOllama EngineMode = "ollama"
)

// Defaults captured as constants so they are documented in one place.
const (
	DefaultClaudeModel  = "sonnet"
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
	Engine        EngineMode
	ClaudeModel   string
	OllamaModel   string // writing model for the Ollama backend
	ExtractModel  string // claim-extraction model for the Ollama backend
	EditModel     string // surgical-review model for the Ollama backend
	ContextLength int
	PredictLength int
	WriteRetries  int
	MaxContinue   int // max length-driven continuations for a single generation
	ForceNew      bool
	Merge         bool // combine every input into one draft instead of queueing
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
		Engine:        EngineAuto,
		ClaudeModel:   env("DRAFT_CLAUDE_MODEL", DefaultClaudeModel),
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
	if v := strings.TrimSpace(os.Getenv("DRAFT_ENGINE")); v != "" {
		c.Engine = EngineMode(v)
	}

	// Flags win over environment.
	if flags.Engine != "" {
		c.Engine = EngineMode(flags.Engine)
	}
	if flags.ClaudeModel != "" {
		c.ClaudeModel = flags.ClaudeModel
	}
	if flags.ContextLength > 0 {
		c.ContextLength = flags.ContextLength
	}
	if flags.PredictLength > 0 {
		c.PredictLength = flags.PredictLength
	}
	c.ForceNew = flags.ForceNew
	c.Merge = flags.Merge
	return c
}

// Flags holds the raw command-line values before they are merged into a Config.
type Flags struct {
	Engine        string
	ClaudeModel   string
	ContextLength int
	PredictLength int
	ForceNew      bool
	Merge         bool
}

func env(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func envInt(name string, fallback, min int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < min {
		return fallback
	}
	return v
}
