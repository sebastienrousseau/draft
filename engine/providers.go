// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// Provider describes how to drive one token-free AI coding-agent CLI in headless
// mode. Every provider authenticates through its own already-logged-in session,
// so draft never handles an API key. A provider is turned into a runnable
// backend by NewSession.
type Provider struct {
	// Name is the stable identifier used on the command line and in the UI.
	Name string
	// Bin is the executable looked up on PATH.
	Bin string
	// Args are fixed arguments placed before the model flag and prompt.
	Args []string
	// ModelFlag, when non-empty, is the flag used to pass a model (e.g.
	// "--model"). Providers without it always use their session's default model.
	ModelFlag string
	// DefaultModel is applied when the user does not override the model and the
	// provider supports ModelFlag.
	DefaultModel string
	// PromptViaStdin sends the prompt on stdin instead of as a positional
	// argument. When false, StdinPlaceholder (if set) is still appended.
	PromptViaStdin bool
	// StdinPlaceholder is appended as a positional argument when the prompt is
	// delivered on stdin and the CLI needs a marker (e.g. "-").
	StdinPlaceholder string
	// Experimental marks a provider whose headless invocation is derived from
	// its --help but whose article output has not been verified end to end. Such
	// providers are skipped by auto-selection unless the user opts in with
	// --experimental; they can always be forced with --engine <name>.
	Experimental bool
	// StreamJSON parses the Claude Code stream-json event format instead of raw
	// text, forwarding token deltas as they arrive for a smooth live preview.
	StreamJSON bool
}

// Providers is the registry of supported session CLIs, in auto-selection
// preference order. The first non-experimental one found on PATH becomes the
// default online backend. Invocations were derived from each CLI's own --help.
//
// claude, copilot, codex, grok, agy, and cursor-agent are verified end to end
// (they return clean Markdown through this abstraction). The rest are
// Experimental: their invocation is correct per --help, but their output has not
// been verified for a full article, so auto-selection skips them unless
// --experimental is set.
//
// PromptViaStdin is set only where stdin delivery was confirmed by running the
// CLI, because a prompt passed as an argument is visible in a process listing
// along with the source text it quotes. Confirmed reading stdin: claude, codex
// ("Reading prompt from stdin..."), cursor-agent (answered a stdin prompt).
// Confirmed NOT to: copilot (ignored a stdin prompt entirely), agy and grok and
// goose (their prompt flags require a value, so there is nothing to read stdin
// into). Unverifiable when checked: amp (out of credits), qwen (no auth
// configured), crush (no provider configured) — their documentation suggests
// stdin support, but guessing here would break a working provider, so they keep
// argument delivery until someone can run them.
var Providers = []Provider{
	{Name: "claude", Bin: "claude", Args: []string{"-p", "--output-format", "stream-json", "--include-partial-messages", "--verbose"}, ModelFlag: "--model", DefaultModel: "sonnet", PromptViaStdin: true, StreamJSON: true},
	{Name: "copilot", Bin: "copilot", Args: []string{"-p"}},
	{Name: "codex", Bin: "codex", Args: []string{"exec"}, ModelFlag: "--model", PromptViaStdin: true},
	{Name: "agy", Bin: "agy", Args: []string{"-p"}, ModelFlag: "--model"},
	{Name: "cursor-agent", Bin: "cursor-agent", Args: []string{"-p", "--output-format", "text"}, ModelFlag: "--model", PromptViaStdin: true},
	{Name: "amp", Bin: "amp", Args: []string{"-x"}, Experimental: true},
	{Name: "crush", Bin: "crush", Args: []string{"run"}, Experimental: true},
	{Name: "goose", Bin: "goose", Args: []string{"run", "--no-session", "-t"}, Experimental: true},
	{Name: "grok", Bin: "grok", Args: []string{"--output-format", "plain", "--single"}},
	{Name: "qwen", Bin: "qwen", Args: []string{"-p"}, Experimental: true},
}

// LookupProvider returns the provider spec for name and whether it exists.
//
// It scans Providers rather than consulting an index built at init: Providers
// is exported and therefore mutable, and an index would silently disagree with
// it the moment a caller appended a provider. Ten entries make the scan free.
func LookupProvider(name string) (Provider, bool) {
	for _, p := range Providers {
		if p.Name == name {
			return p, true
		}
	}
	return Provider{}, false
}

// ProviderNames returns every registered provider name in preference order.
func ProviderNames() []string {
	names := make([]string, len(Providers))
	for i, p := range Providers {
		names[i] = p.Name
	}
	return names
}

// available reports whether the provider's CLI is installed on PATH. It is a
// variable so tests can simulate installed or missing binaries.
var available = func(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// IsAvailable reports whether the provider binary for name (or ollama) is installed on PATH.
func IsAvailable(name string) bool {
	if name == "auto" {
		return true
	}
	if name == "ollama" {
		return available("ollama")
	}
	if p, ok := LookupProvider(name); ok {
		return available(p.Bin)
	}
	return false
}

// FirstAvailableProvider returns the first registered provider whose CLI is
// installed, in preference order. Experimental providers are considered only
// when includeExperimental is true.
func FirstAvailableProvider(includeExperimental bool) (Provider, bool) {
	for _, p := range Providers {
		if p.Experimental && !includeExperimental {
			continue
		}
		if available(p.Bin) {
			return p, true
		}
	}
	return Provider{}, false
}

var dialTimeout = net.DialTimeout

// startCommand builds the background server command; a variable so the
// reaping path can be exercised without a real Ollama install.
var startCommand = exec.Command

// IsOnline probes public DNS endpoints with a short timeout to check for network connectivity.
var IsOnline = func() bool {
	conn, err := dialTimeout("tcp", "1.1.1.1:53", 300*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		return true
	}
	conn2, err2 := dialTimeout("tcp", "8.8.8.8:53", 300*time.Millisecond)
	if err2 == nil {
		_ = conn2.Close()
		return true
	}
	return false
}

// IsOllamaRunning reports whether a local Ollama server is responding at host.
func IsOllamaRunning(host string) bool {
	if host == "" {
		host = "http://127.0.0.1:11434"
	}
	url := strings.TrimRight(host, "/") + "/api/tags"
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 500
}

// EnsureOllamaRunning checks if Ollama is running at host. If not, it attempts to launch
// `ollama serve` in the background and waits up to 3 seconds for it to become responsive.
func EnsureOllamaRunning(host string) error {
	if IsOllamaRunning(host) {
		return nil
	}
	if !available("ollama") {
		return fmt.Errorf("ollama is not installed on PATH; please install Ollama from https://ollama.com")
	}
	cmd := startCommand("ollama", "serve")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start 'ollama serve': %w", err)
	}
	// Reap the child when it eventually exits. draft does not own the
	// server's lifetime — it is deliberately left running for the next run —
	// but a started process that is never waited on becomes a zombie held by
	// this process for as long as it lives.
	go func() { _ = cmd.Wait() }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		if IsOllamaRunning(host) {
			return nil
		}
	}
	return fmt.Errorf("started 'ollama serve' but server at %s did not respond within 3 seconds", host)
}
