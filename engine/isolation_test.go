// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/config"
)

// toolGrantingFlags are invocations that hand an agent the ability to act on
// the machine: run commands, write files, fetch URLs, auto-approve MCP servers,
// or widen the directories it may touch.
//
// draft only ever asks a provider for text, and every prompt it sends carries
// verbatim content from a third-party document. An invocation that grants tools
// therefore turns a crafted PDF into an execution path on the user's machine
// with the user's credentials. No entry in Providers may carry one.
var toolGrantingFlags = []string{
	"--allow-all-tools", "--allow-all", "--allow-all-paths", "--allow-all-urls",
	"--allow-all-mcp-server-instructions", "--approve-mcps",
	"--dangerously-skip-permissions", "--allow-dangerously-skip-permissions",
	"--dangerously-bypass-approvals-and-sandbox", "--dangerously-bypass-hook-trust",
	"--force", "--yolo", "-y", "--full-auto", "--auto-approve", "--no-sandbox",
	"--allow-tool", "--allowedTools", "--allowed-tools", "--add-dir",
}

// toolGrantingValues are flags that are harmless with one value and grant
// tools with another, so a list of flag names alone cannot catch them.
var toolGrantingValues = map[string][]string{
	"--permission-mode": {"bypassPermissions", "acceptEdits", "auto", "dontAsk"},
	"--sandbox":         {"danger-full-access", "workspace-write"},
	"-s":                {"danger-full-access", "workspace-write"},
	"--approval-mode":   {"yolo", "auto_edit"},
}

// flagName is the flag an argument sets, so "--yolo=true" is caught as
// "--yolo". Comparing whole arguments missed every --flag=value spelling.
func flagName(arg string) string {
	name, _, _ := strings.Cut(arg, "=")
	return name
}

// flagValue is the value an argument sets for its flag, whether written
// "--flag=value" or as the next argument.
func flagValue(args []string, i int) string {
	if _, v, ok := strings.Cut(args[i], "="); ok {
		return v
	}
	if i+1 < len(args) {
		return args[i+1]
	}
	return ""
}

func grantsTools(args []string, i int) bool {
	name := flagName(args[i])
	if slices.Contains(toolGrantingFlags, name) {
		return true
	}
	return slices.Contains(toolGrantingValues[name], flagValue(args, i))
}

func TestNoProviderGrantsTools(t *testing.T) {
	for _, p := range Providers() {
		for i, arg := range p.Args {
			if grantsTools(p.Args, i) {
				t.Errorf("provider %q is invoked with %s, which grants the agent tools; "+
					"draft feeds it untrusted document text and only ever needs text back",
					p.Name, arg)
			}
		}
	}
}

// The denylist must catch the spellings it used to miss.
func TestGrantsToolsCatchesEverySpelling(t *testing.T) {
	for _, args := range [][]string{
		{"--yolo=true"},
		{"--allow-all-tools=1"},
		{"--permission-mode", "bypassPermissions"},
		{"--permission-mode=acceptEdits"},
		{"--sandbox", "danger-full-access"},
		{"-s", "workspace-write"},
		{"--approval-mode=yolo"},
		{"--add-dir", "/"},
	} {
		if !grantsTools(args, 0) {
			t.Errorf("grantsTools(%q) = false", args)
		}
	}
	for _, args := range [][]string{
		{"--permission-mode", "plan"},
		{"--sandbox", "read-only"},
		{"--tools", ""},
	} {
		if grantsTools(args, 0) {
			t.Errorf("grantsTools(%q) = true for a restricting flag", args)
		}
	}
}

// Passing no tool-granting flag is not the same as granting no tools: a CLI
// left to its defaults loads the user's own tools, hooks and MCP servers. For
// each provider that can be told otherwise, these are the restrictions its
// invocation must keep. Each was checked against the CLI by asking it to read a
// canary file outside its working directory.
var requiredRestrictions = map[string][][]string{
	"claude": {
		{"--tools", ""}, // no built-in tools at all
		{"--strict-mcp-config"},
		{"--safe-mode"}, // no hooks, plugins, MCP servers or CLAUDE.md
	},
	"codex": {
		{"--ignore-user-config"},
		{"--disable", "shell_tool"},
		{"--disable", "unified_exec"},
		{"-c", `web_search="disabled"`},
		{"-c", `default_permissions="draft"`},
	},
	"agy": {
		{"--disable-slash-commands"},
		{"--settings", `{"allowNonWorkspaceAccess":false,"enableTerminalSandbox":true,"permissions":{"deny":["command(*)","read_file(*)","write_file(*)","read_url(*)","mcp(*)"]}}`},
	},
	"copilot": {
		{"--disable-builtin-mcps"},
		{"--no-custom-instructions"},
	},
}

func TestProvidersKeepTheirRestrictions(t *testing.T) {
	for name, wants := range requiredRestrictions {
		p, ok := LookupProvider(name)
		if !ok {
			t.Errorf("provider %q is not registered", name)
			continue
		}
		for _, want := range wants {
			if !containsRun(p.Args, want) {
				t.Errorf("provider %q has lost %q from its invocation: %q", name, want, p.Args)
			}
		}
	}
}

// Auto mode picks a provider without the user naming it, so it may only pick
// one whose tools were taken away and checked against the canary, or an ACP
// agent, whose tool turns fail closed. Anything else is experimental: still
// usable by name, never chosen for the user.
func TestAutoModeUsesOnlyRestrictedProviders(t *testing.T) {
	for _, p := range Providers() {
		if p.Experimental || p.ACP {
			continue
		}
		if _, ok := requiredRestrictions[p.Name]; !ok {
			t.Errorf("auto mode can pick %q, which keeps its own tools; restrict it or mark it Experimental", p.Name)
		}
	}
}

// containsRun reports whether want appears in args as consecutive arguments,
// so a flag is matched with its own value and not one from elsewhere.
func containsRun(args, want []string) bool {
	for i := 0; i+len(want) <= len(args); i++ {
		if slices.Equal(args[i:i+len(want)], want) {
			return true
		}
	}
	return false
}

// captureExec records the command Session.Generate built, then runs the helper
// so Generate still completes normally.
func captureExec(got **exec.Cmd) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := append([]string{"-test.run=TestHelperProcess", "--", name}, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "HELPER_MODE=default")
		*got = cmd
		return cmd
	}
}

// A provider started in the user's working directory loads whatever agent
// configuration lives there and widens the blast radius of a crafted source.
func TestSessionRunsInAnIsolatedEmptyDirectory(t *testing.T) {
	var captured *exec.Cmd
	orig := execCommand
	execCommand = captureExec(&captured)
	defer func() { execCommand = orig }()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	s, _ := NewSession("claude", config.Config{})
	if _, err := s.Generate(context.Background(), Request{Prompt: "hi"}); err != nil {
		t.Fatal(err)
	}
	if captured.Dir == "" {
		t.Fatal("Session.Generate left cmd.Dir unset; the provider would inherit the working directory")
	}
	if captured.Dir == wd {
		t.Errorf("Session.Generate ran the provider in the working directory %q", wd)
	}
	// The directory is removed once the call returns, so nothing accumulates.
	if _, err := os.Stat(captured.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("sandbox directory %q survived the call", captured.Dir)
	}
}

func TestSessionStripsDraftConfigFromTheEnvironment(t *testing.T) {
	t.Setenv("DRAFT_ENGINE", "ollama")
	t.Setenv("DRAFT_WRITE_MODEL", "secret-model")
	t.Setenv("KEEP_ME", "yes")

	var captured *exec.Cmd
	orig := execCommand
	execCommand = captureExec(&captured)
	defer func() { execCommand = orig }()

	s, _ := NewSession("claude", config.Config{})
	if _, err := s.Generate(context.Background(), Request{Prompt: "hi"}); err != nil {
		t.Fatal(err)
	}
	for _, kv := range captured.Env {
		if strings.HasPrefix(kv, "DRAFT_") {
			t.Errorf("provider environment carried draft's own config: %q", kv)
		}
	}
	var kept bool
	for _, kv := range captured.Env {
		if kv == "KEEP_ME=yes" {
			kept = true
		}
	}
	if !kept {
		t.Error("sessionEnv dropped an unrelated variable; provider auth lives in the environment")
	}
}

// A run must fail loudly rather than silently fall back to the user's working
// directory, which is the condition the isolation exists to prevent.
func TestSessionFailsWhenTheSandboxCannotBeCreated(t *testing.T) {
	origMk := mkTempDir
	mkTempDir = func(string, string) (string, error) { return "", errors.New("disk full") }
	defer func() { mkTempDir = origMk }()

	origExec := execCommand
	execCommand = fakeExec("default")
	defer func() { execCommand = origExec }()

	s, _ := NewSession("claude", config.Config{})
	_, err := s.Generate(context.Background(), Request{Prompt: "hi"})
	if err == nil {
		t.Fatal("Generate succeeded despite failing to create an isolated directory")
	}
	if !strings.Contains(err.Error(), "isolated working directory") {
		t.Errorf("error should name the cause, got %q", err)
	}
}

// A positional prompt puts the verbatim source text into the process listing.
// Every provider must therefore deliver it on stdin or through a private file;
// the exceptions are the CLIs whose own --help offers neither.
func TestPromptStaysOutOfArgvWhereverPossible(t *testing.T) {
	// These CLIs read the prompt only as a positional argument, and offer
	// no stdin, prompt-file, ACP or stream-json path to avoid it. copilot uses
	// ACP and agy uses its stream-json stdin turn protocol, so both are off the
	// list; amp, crush and qwen remain until one of those appears.
	argvOnly := map[string]bool{"amp": true, "crush": true, "qwen": true}
	for _, p := range Providers() {
		// An ACP agent receives the prompt inside a JSON-RPC frame on its
		// stdin; argv only ever carries the fixed start-up arguments.
		if p.ACP || p.StreamJSONInput || p.PromptViaStdin || p.PromptFileFlag != "" || argvOnly[p.Name] {
			continue
		}
		t.Errorf("provider %q delivers the prompt in argv but is not on the exception list", p.Name)
	}
}

func TestPromptFileIsStagedPrivatelyAndPassedByPath(t *testing.T) {
	var captured *exec.Cmd
	orig := execCommand
	execCommand = captureExec(&captured)
	defer func() { execCommand = orig }()

	s, _ := NewSession("grok", config.Config{})
	if _, err := s.Generate(context.Background(), Request{Prompt: "secret source text"}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(captured.Args, "\x1f")
	if strings.Contains(joined, "secret source text") {
		t.Errorf("prompt leaked into argv: %q", joined)
	}
	if !strings.Contains(joined, "--prompt-file") {
		t.Errorf("prompt file flag missing from argv: %q", joined)
	}
}

func TestPromptFileFailureIsReported(t *testing.T) {
	origMk := mkTempDir
	// A directory that does not exist makes the staging write fail without
	// needing an unwritable filesystem. Built from t.TempDir so the path is
	// real for the platform rather than a POSIX literal.
	missing := filepath.Join(t.TempDir(), "never-created")
	mkTempDir = func(string, string) (string, error) { return missing, nil }
	defer func() { mkTempDir = origMk }()

	origExec := execCommand
	execCommand = fakeExec("default")
	defer func() { execCommand = origExec }()

	s, _ := NewSession("grok", config.Config{})
	_, err := s.Generate(context.Background(), Request{Prompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "stage the prompt") {
		t.Errorf("expected a staging error, got %v", err)
	}
}
