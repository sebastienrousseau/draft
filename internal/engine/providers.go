// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import "os/exec"

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
// claude, copilot, codex, and grok are verified end to end (they return clean
// Markdown through this abstraction). The rest are Experimental: their invocation
// is correct per --help, but their output has not been verified for a full
// article, so auto-selection skips them unless --experimental is set.
var Providers = []Provider{
	{Name: "claude", Bin: "claude", Args: []string{"-p", "--output-format", "stream-json", "--include-partial-messages", "--verbose"}, ModelFlag: "--model", DefaultModel: "sonnet", PromptViaStdin: true, StreamJSON: true},
	{Name: "copilot", Bin: "copilot", Args: []string{"-p", "--allow-all-tools"}},
	{Name: "codex", Bin: "codex", Args: []string{"exec"}, ModelFlag: "--model"},
	{Name: "gemini", Bin: "gemini", Args: []string{"-p"}, ModelFlag: "--model", Experimental: true},
	{Name: "cursor-agent", Bin: "cursor-agent", Args: []string{"-p", "--output-format", "text"}, ModelFlag: "--model", Experimental: true},
	{Name: "amp", Bin: "amp", Args: []string{"-x"}, Experimental: true},
	{Name: "crush", Bin: "crush", Args: []string{"run"}, Experimental: true},
	{Name: "goose", Bin: "goose", Args: []string{"run", "--no-session", "-t"}, Experimental: true},
	{Name: "grok", Bin: "grok", Args: []string{"--output-format", "plain", "--single"}},
	{Name: "qwen", Bin: "qwen", Args: []string{"-p"}, Experimental: true},
}

// providerByName indexes Providers for O(1) lookup.
var providerByName = func() map[string]Provider {
	m := make(map[string]Provider, len(Providers))
	for _, p := range Providers {
		m[p.Name] = p
	}
	return m
}()

// LookupProvider returns the provider spec for name and whether it exists.
func LookupProvider(name string) (Provider, bool) {
	p, ok := providerByName[name]
	return p, ok
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
