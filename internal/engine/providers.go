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
}

// Providers is the registry of supported session CLIs, in auto-selection
// preference order. The first one found on PATH becomes the default online
// backend. Invocations were derived from each CLI's own --help.
var Providers = []Provider{
	{Name: "claude", Bin: "claude", Args: []string{"-p", "--output-format", "text"}, ModelFlag: "--model", DefaultModel: "sonnet", PromptViaStdin: true},
	{Name: "codex", Bin: "codex", Args: []string{"exec"}, ModelFlag: "--model"},
	{Name: "gemini", Bin: "gemini", Args: []string{"-p"}, ModelFlag: "--model"},
	{Name: "copilot", Bin: "copilot", Args: []string{"-p", "--allow-all-tools"}},
	{Name: "cursor-agent", Bin: "cursor-agent", Args: []string{"-p", "--output-format", "text"}, ModelFlag: "--model"},
	{Name: "amp", Bin: "amp", Args: []string{"-x"}},
	{Name: "crush", Bin: "crush", Args: []string{"run"}},
	{Name: "goose", Bin: "goose", Args: []string{"run", "--no-session", "-t"}},
	{Name: "grok", Bin: "grok", Args: []string{"--output-format", "plain", "--single"}},
	{Name: "qwen", Bin: "qwen", Args: []string{"-p"}},
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
// installed, in preference order.
func FirstAvailableProvider() (Provider, bool) {
	for _, p := range Providers {
		if available(p.Bin) {
			return p, true
		}
	}
	return Provider{}, false
}
