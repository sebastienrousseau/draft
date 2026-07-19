// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/internal/config"
)

// TestHelperProcess is not a real test: it is re-executed as the fake provider
// CLI by fakeExec. Its behaviour is driven by environment variables so a single
// helper can play a well-behaved CLI, a failing CLI, or an echo of its input.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	switch os.Getenv("HELPER_MODE") {
	case "fail":
		os.Stderr.WriteString("boom: not logged in\nsecond line\n")
		os.Exit(3)
	case "echo-args":
		// Print the arguments after the "--" separator so tests can assert them.
		args := os.Args
		for i, a := range args {
			if a == "--" {
				os.Stdout.WriteString(strings.Join(args[i+1:], "\x1f"))
				break
			}
		}
	case "echo-stdin":
		buf := new(strings.Builder)
		_, _ = io.Copy(buf, os.Stdin)
		os.Stdout.WriteString("STDIN:" + buf.String())
	default:
		os.Stdout.WriteString("# Title\n\nbody text.")
	}
	os.Exit(0)
}

// fakeExec returns an *exec.Cmd that re-runs this test binary as the helper
// process, in the given mode.
func fakeExec(mode string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := append([]string{"-test.run=TestHelperProcess", "--", name}, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "HELPER_MODE="+mode)
		return cmd
	}
}

func withExec(mode string, fn func()) {
	orig := execCommand
	execCommand = fakeExec(mode)
	defer func() { execCommand = orig }()
	fn()
}

func TestSessionGenerateSuccess(t *testing.T) {
	withExec("default", func() {
		s, ok := NewSession("claude", config.Config{})
		if !ok {
			t.Fatal("claude provider should exist")
		}
		var streamed strings.Builder
		res, err := s.Generate(context.Background(), Request{
			Kind:    KindWrite,
			Prompt:  "write it",
			OnChunk: func(c string) { streamed.WriteString(c) },
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(res.Text, "# Title") {
			t.Errorf("unexpected text: %q", res.Text)
		}
		if streamed.Len() == 0 {
			t.Error("OnChunk was never called")
		}
	})
}

func TestSessionGenerateError(t *testing.T) {
	withExec("fail", func() {
		s, _ := NewSession("claude", config.Config{})
		_, err := s.Generate(context.Background(), Request{Prompt: "x"})
		if err == nil {
			t.Fatal("expected error from failing CLI")
		}
		// Only the first stderr line is surfaced, prefixed with the provider.
		if !strings.Contains(err.Error(), "claude: boom") {
			t.Errorf("error should carry provider + first stderr line, got %q", err)
		}
	})
}

func TestSessionPromptViaStdin(t *testing.T) {
	withExec("echo-stdin", func() {
		s, _ := NewSession("claude", config.Config{}) // claude uses stdin
		res, err := s.Generate(context.Background(), Request{Prompt: "hello prompt"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(res.Text, "STDIN:hello prompt") {
			t.Errorf("prompt should be delivered on stdin, got %q", res.Text)
		}
	})
}

func TestSessionPromptViaArg(t *testing.T) {
	withExec("echo-args", func() {
		s, _ := NewSession("codex", config.Config{}) // codex uses a positional arg
		res, err := s.Generate(context.Background(), Request{Prompt: "arg prompt"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(res.Text, "arg prompt") {
			t.Errorf("prompt should be the final arg, got %q", res.Text)
		}
		if !strings.Contains(res.Text, "exec") {
			t.Errorf("provider args should be present, got %q", res.Text)
		}
	})
}

func TestSessionModelFlag(t *testing.T) {
	withExec("echo-args", func() {
		s, _ := NewSession("claude", config.Config{Model: "opus"})
		res, _ := s.Generate(context.Background(), Request{Prompt: "p"})
		if !strings.Contains(res.Text, "--model\x1fopus") {
			t.Errorf("model flag should be passed, got %q", res.Text)
		}
	})
	withExec("echo-args", func() {
		// A provider without a model flag must not receive one.
		s, _ := NewSession("crush", config.Config{Model: "whatever"})
		res, _ := s.Generate(context.Background(), Request{Prompt: "p"})
		if strings.Contains(res.Text, "--model") {
			t.Errorf("crush has no model flag; none should be passed, got %q", res.Text)
		}
	})
}

func TestNewSessionUnknown(t *testing.T) {
	if _, ok := NewSession("nope", config.Config{}); ok {
		t.Error("unknown provider should not resolve")
	}
}

func TestSessionDefaultModel(t *testing.T) {
	s, _ := NewSession("claude", config.Config{}) // no override -> provider default
	if s.model != "sonnet" {
		t.Errorf("expected default model sonnet, got %q", s.model)
	}
}

func TestFirstLineHelper(t *testing.T) {
	if firstLine("only line") != "only line" {
		t.Error("no-newline case")
	}
	if firstLine("first\nsecond") != "first" {
		t.Error("multi-line case")
	}
}

func TestSessionStdinPlaceholder(t *testing.T) {
	withExec("echo-args", func() {
		// A provider that streams the prompt on stdin but needs a positional
		// placeholder marker.
		s := &Session{provider: Provider{Name: "t", Bin: "claude", PromptViaStdin: true, StdinPlaceholder: "-"}}
		res, err := s.Generate(context.Background(), Request{Prompt: "p"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(res.Text, "-") {
			t.Errorf("stdin placeholder should be appended, got %q", res.Text)
		}
	})
}

func TestSessionContextCancelled(t *testing.T) {
	withExec("default", func() {
		s, _ := NewSession("claude", config.Config{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancelled before the call
		if _, err := s.Generate(ctx, Request{Prompt: "p"}); err == nil {
			t.Error("a cancelled context should abort the session call")
		}
	})
}
