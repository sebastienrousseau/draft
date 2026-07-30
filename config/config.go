// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package config resolves runtime configuration from command-line flags,
// environment variables, and sensible defaults, in that order of precedence.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Engine selection sentinels. Any other value names a specific session provider
// (claude, codex, agy, copilot, cursor-agent, amp, crush, goose, grok, ...).
const (
	// EngineAuto picks the first installed session provider and falls back to
	// Ollama if a call fails (offline). This is the default.
	EngineAuto = "auto"
	// EngineOllama forces the local Ollama backend and never touches a session
	// CLI or the network.
	EngineOllama = "ollama"
)

// Defaults captured as constants so they are documented in one place.
//
// All three Ollama stages default to gemma3:4b. On a memory-constrained machine
// a single shared model means the server never swaps a second 4B model in and
// out between extraction and writing. gemma also follows the writing brief far
// more closely than qwen3:4b, which tended to overshoot the word budget and leak
// its own planning text into the article.
const (
	DefaultOllamaModel        = "gemma3:4b"
	DefaultExtractModel       = "gemma3:4b"
	DefaultEditModel          = "gemma3:4b"
	DefaultContextLen         = 8192
	DefaultPredictLen         = 6000
	DefaultWriteRetries       = 2
	DefaultMaxContinue        = 3
	DefaultExtractConcurrency = 4
	OllamaHost                = "http://127.0.0.1:11434"
	FocusBlock                = 25 * time.Minute
	// DefaultCallTimeout bounds a single generation call. Generation is slow —
	// a full article on a small local model can run for many minutes — so the
	// ceiling is generous; its job is only to stop a wedged provider CLI or a
	// black-holed HTTP connection from hanging the run forever. Set
	// DRAFT_CALL_TIMEOUT=0 to disable it.
	DefaultCallTimeout = 30 * time.Minute
)

// Upper bounds for the numeric environment variables. A floor alone leaves a
// fat-fingered or hostile value free to exhaust memory or spawn a process per
// section, so every tunable is clamped at both ends.
const (
	maxContextLen         = 1 << 20
	maxPredictLen         = 1 << 20
	maxWriteRetries       = 10
	maxContinue           = 20
	maxExtractConcurrency = 32
	maxCallTimeoutSecs    = 24 * 60 * 60
)

// Config is the fully-resolved run configuration shared across packages.
type Config struct {
	Engine       string // "auto", "ollama", or a session provider name
	Model        string // session-provider model override ("" = provider default)
	OllamaModel  string // writing model for the Ollama backend
	ExtractModel string // claim-extraction model for the Ollama backend
	EditModel    string // surgical-review model for the Ollama backend

	ContextLength      int
	PredictLength      int
	WriteRetries       int
	MaxContinue        int // max length-driven continuations for a single generation
	ExtractConcurrency int // parallel claim-extraction workers (session engines only)
	ForceNew           bool
	Merge              bool // combine every input into one draft instead of queueing
	KeepArtifacts      bool // keep prompt/ledger files beside a successful draft
	Experimental       bool // let auto-selection consider experimental providers
	OllamaHost         string

	// CallTimeout bounds a single generation call (0 = no timeout).
	CallTimeout time.Duration

	HomeDir    string
	SourcesDir string
	DraftsDir  string

	// Warnings records configuration problems that were recovered from rather
	// than fatal: an unreadable home directory, an out-of-range tunable, a
	// non-loopback Ollama host. The CLI prints them to stderr so a silent
	// fallback is never mistaken for the configuration the user asked for.
	Warnings []string
}

// Load builds a Config from defaults, overlays environment variables, then
// overlays the already-parsed flag values passed in from the caller.
func Load(flags Flags) Config {
	var warnings []string
	warn := func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}

	home := resolveHome(warn)
	c := Config{
		Engine:             env("DRAFT_ENGINE", EngineAuto),
		Model:              env("DRAFT_MODEL_SESSION", env("DRAFT_CLAUDE_MODEL", "")),
		OllamaModel:        env("DRAFT_WRITE_MODEL", env("DRAFT_MODEL", DefaultOllamaModel)),
		ExtractModel:       env("DRAFT_EXTRACT_MODEL", env("DRAFT_MODEL", DefaultExtractModel)),
		EditModel:          env("DRAFT_EDIT_MODEL", env("DRAFT_MODEL", DefaultEditModel)),
		ContextLength:      envInt(warn, "DRAFT_NUM_CTX", DefaultContextLen, 512, maxContextLen),
		PredictLength:      envInt(warn, "DRAFT_NUM_PREDICT", DefaultPredictLen, 1024, maxPredictLen),
		WriteRetries:       envInt(warn, "DRAFT_WRITE_RETRIES", DefaultWriteRetries, 0, maxWriteRetries),
		MaxContinue:        envInt(warn, "DRAFT_MAX_CONTINUE", DefaultMaxContinue, 0, maxContinue),
		ExtractConcurrency: envInt(warn, "DRAFT_EXTRACT_CONCURRENCY", DefaultExtractConcurrency, 1, maxExtractConcurrency),
		OllamaHost:         resolveOllamaHost(warn),
		CallTimeout:        resolveCallTimeout(warn),
		HomeDir:            home,
		SourcesDir:         filepath.Join(home, "Drop", "Drafts", "Sources"),
		DraftsDir:          filepath.Join(home, "Drop", "Drafts"),
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
	c.Warnings = warnings
	return c
}

// resolveHome returns the directory the Sources and Drafts trees hang off.
// os.UserHomeDir fails when HOME is unset — routine under systemd, cron and in
// containers, which is exactly where draft's --json mode is meant to run. The
// old code discarded that error, leaving HomeDir empty and turning SourcesDir
// and DraftsDir into RELATIVE paths, so drafts were silently written into
// whatever directory the process happened to start in. Fall back to an
// absolute path instead, and say so.
func resolveHome(warn func(string, ...any)) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return home
	}
	if wd, wdErr := os.Getwd(); wdErr == nil && wd != "" {
		warn("home directory unavailable (%v); using the working directory %s for Sources and Drafts", err, wd)
		return wd
	}
	tmp := os.TempDir()
	warn("home directory and working directory both unavailable (%v); using %s for Sources and Drafts", err, tmp)
	return tmp
}

// resolveOllamaHost validates OLLAMA_HOST. An unparseable or non-HTTP value is
// rejected in favour of the default rather than being concatenated into a
// request URL, and a host that is not loopback is reported: draft is documented
// as working offline, so a remote Ollama means every prompt — including the
// verbatim source text — leaves the machine.
func resolveOllamaHost(warn func(string, ...any)) string {
	raw := env("OLLAMA_HOST", OllamaHost)
	if raw == OllamaHost {
		return raw
	}
	// A bare "host:port" is what the ollama CLI itself accepts; normalise it
	// rather than rejecting a form users reasonably expect to work.
	probe := raw
	if !strings.Contains(probe, "://") {
		probe = "http://" + probe
	}
	u, err := url.Parse(probe)
	if err != nil || u.Host == "" {
		warn("OLLAMA_HOST %q is not a valid URL; using %s", raw, OllamaHost)
		return OllamaHost
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		warn("OLLAMA_HOST %q does not use http or https; using %s", raw, OllamaHost)
		return OllamaHost
	}
	if !isLoopback(u.Hostname()) {
		warn("OLLAMA_HOST %q is not loopback: prompts and source text will be sent to %s", raw, u.Host)
	}
	return strings.TrimRight(u.String(), "/")
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// resolveCallTimeout reads DRAFT_CALL_TIMEOUT as a whole number of seconds.
// An explicit 0 disables the bound.
func resolveCallTimeout(warn func(string, ...any)) time.Duration {
	raw := strings.TrimSpace(os.Getenv("DRAFT_CALL_TIMEOUT"))
	if raw == "" {
		return DefaultCallTimeout
	}
	secs, err := strconv.Atoi(raw)
	if err != nil || secs < 0 || secs > maxCallTimeoutSecs {
		warn("DRAFT_CALL_TIMEOUT %q is not a whole number of seconds in [0, %d]; using %s", raw, maxCallTimeoutSecs, DefaultCallTimeout)
		return DefaultCallTimeout
	}
	return time.Duration(secs) * time.Second
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

// envInt reads a numeric environment variable, clamped at both ends. A value
// outside [minValue, maxValue] falls back to the default and is reported: a
// silently ignored tunable is worse than a rejected one, because the user
// believes it took effect.
func envInt(warn func(string, ...any), name string, fallback, minValue, maxValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		warn("%s=%q is not a number; using %d", name, raw, fallback)
		return fallback
	}
	if v < minValue || v > maxValue {
		warn("%s=%d is outside [%d, %d]; using %d", name, v, minValue, maxValue, fallback)
		return fallback
	}
	return v
}
