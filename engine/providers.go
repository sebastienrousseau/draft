// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
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
	// PromptFileFlag, when non-empty, writes the prompt to a private file
	// inside the call's sandbox directory and passes its path with this flag
	// (e.g. "--prompt-file"). It is the second-best delivery after stdin, and
	// far better than argv: a positional prompt carries the verbatim source
	// text into the process listing, where any local user can read it.
	PromptFileFlag string
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

// defaultProviders is the built-in registry of supported session CLIs, in
// auto-selection preference order. The first non-experimental one found on PATH becomes the
// default online backend. Invocations were derived from each CLI's own --help.
//
// claude, copilot, codex, grok, agy, and cursor-agent are verified end to end
// (they return clean Markdown through this abstraction). The rest are
// Experimental: their invocation is correct per --help, but their output has not
// been verified for a full article, so auto-selection skips them unless
// --experimental is set.
//
// The prompt is kept out of argv wherever the CLI allows it, because a
// positional prompt is visible in a process listing — along with the verbatim
// source text it quotes — to any other user on the machine.
//
// Confirmed reading stdin: claude, codex ("Reading prompt from stdin..."),
// cursor-agent (answered a stdin prompt), and goose, whose `run -i <FILE>`
// documents "Use - for stdin" (an earlier note here recorded goose as
// argv-only, which its own --help contradicts).
//
// Confirmed to accept a prompt file: grok, via `--prompt-file <PATH>`
// ("Single-turn prompt from a file").
//
// Confirmed NOT to read stdin: copilot (ignored a stdin prompt entirely).
// agy offers only --input-format stream-json, an NDJSON turn protocol rather
// than a plain prompt, so it keeps argv until that is implemented.
// Unverifiable when checked: amp (out of credits), qwen (no auth configured),
// crush (no provider configured) — their documentation suggests stdin support,
// but guessing here would break a working provider, so they keep argument
// delivery until someone can run them.
func defaultProviders() []Provider {
	return []Provider{
		{Name: "claude", Bin: "claude", Args: []string{"-p", "--output-format", "stream-json", "--include-partial-messages", "--verbose"}, ModelFlag: "--model", DefaultModel: "sonnet", PromptViaStdin: true, StreamJSON: true},
		{Name: "copilot", Bin: "copilot", Args: []string{"-p"}},
		{Name: "codex", Bin: "codex", Args: []string{"exec"}, ModelFlag: "--model", PromptViaStdin: true},
		{Name: "agy", Bin: "agy", Args: []string{"-p"}, ModelFlag: "--model"},
		{Name: "cursor-agent", Bin: "cursor-agent", Args: []string{"-p", "--output-format", "text"}, ModelFlag: "--model", PromptViaStdin: true},
		{Name: "amp", Bin: "amp", Args: []string{"-x"}, Experimental: true},
		{Name: "crush", Bin: "crush", Args: []string{"run"}, Experimental: true},
		{Name: "goose", Bin: "goose", Args: []string{"run", "--no-session", "-i", "-"}, PromptViaStdin: true, Experimental: true},
		{Name: "grok", Bin: "grok", Args: []string{"--output-format", "plain", "--single"}, PromptFileFlag: "--prompt-file"},
		{Name: "qwen", Bin: "qwen", Args: []string{"-p"}, Experimental: true},
	}
}

// registry holds the provider table behind a lock.
//
// It used to be an exported slice, and LookupProvider's comment documented
// that callers may append to it. For a package meant to be embedded that is a
// data race the detector can only catch by luck: a consumer registering a
// provider while a run is in flight races every Chain, LookupProvider and
// FirstAvailableProvider call at once. For a library the public surface is the
// product, so the table is reached only through these accessors.
var registry = struct {
	mu   sync.RWMutex
	list []Provider
}{list: defaultProviders()}

// Providers returns the registered session CLIs in auto-selection preference
// order. The result is a copy: mutating it does not change the registry, and
// Register is the supported way to extend it.
func Providers() []Provider {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return append([]Provider(nil), registry.list...)
}

// Register adds a provider, or replaces the entry with the same name. A
// provider needs at least a name and a binary to be runnable; anything less
// would fail later, at the point of a generation call, with a far worse
// message.
func Register(p Provider) error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("provider needs a name")
	}
	if strings.TrimSpace(p.Bin) == "" {
		return fmt.Errorf("provider %q needs a binary to look up on PATH", p.Name)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for i := range registry.list {
		if registry.list[i].Name == p.Name {
			registry.list[i] = p
			return nil
		}
	}
	registry.list = append(registry.list, p)
	return nil
}

// ResetProviders restores the built-in table, discarding anything registered.
// It exists so a test that registers a provider can undo it.
func ResetProviders() {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.list = defaultProviders()
}

// LookupProvider returns the provider spec for name and whether it exists.
//
// It scans rather than consulting an index built at init: the registry is
// extensible, and an index would silently disagree with it the moment a caller
// registered a provider. Ten entries make the scan free.
func LookupProvider(name string) (Provider, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	for _, p := range registry.list {
		if p.Name == name {
			return p, true
		}
	}
	return Provider{}, false
}

// ProviderNames returns every registered provider name in preference order.
func ProviderNames() []string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	names := make([]string, len(registry.list))
	for i, p := range registry.list {
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
	for _, p := range Providers() {
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
