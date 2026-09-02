// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/config"
)

// toolGrantingFlags are invocations that hand an agent the ability to act on
// the machine: run commands, write files, fetch URLs, auto-approve MCP servers.
//
// draft only ever asks a provider for text, and every prompt it sends carries
// verbatim content from a third-party document. An invocation that grants tools
// therefore turns a crafted PDF into an execution path on the user's machine
// with the user's credentials. No entry in Providers may carry one.
var toolGrantingFlags = []string{
	"--allow-all-tools", "--allow-all", "--allow-all-paths", "--allow-all-urls",
	"--allow-all-mcp-server-instructions", "--approve-mcps",
	"--dangerously-skip-permissions", "--allow-dangerously-skip-permissions",
	"--force", "--yolo", "--full-auto", "--auto-approve", "--no-sandbox",
}

func TestNoProviderGrantsTools(t *testing.T) {
	for _, p := range Providers {
		for _, arg := range p.Args {
			for _, bad := range toolGrantingFlags {
				if arg == bad {
					t.Errorf("provider %q is invoked with %s, which grants the agent tools; "+
						"draft feeds it untrusted document text and only ever needs text back",
						p.Name, bad)
				}
			}
		}
	}
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

func TestSessionEnvKeepsEverythingElse(t *testing.T) {
	in := []string{"PATH=/bin", "DRAFT_MODEL=x", "HOME=/home/u", "DRAFTY=keep"}
	got := sessionEnv(in)
	want := []string{"PATH=/bin", "HOME=/home/u", "DRAFTY=keep"}
	if strings.Join(got, "\x1f") != strings.Join(want, "\x1f") {
		t.Errorf("sessionEnv() = %v, want %v", got, want)
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
