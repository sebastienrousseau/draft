// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/sebastienrousseau/draft/config"
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
	case "fail-empty":
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
	case "stream-json":
		os.Stdout.WriteString(`{"type":"system","subtype":"init"}` + "\n")
		os.Stdout.WriteString(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"# Hi"}}}` + "\n")
		os.Stdout.WriteString(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":" there."}}}` + "\n")
		os.Stdout.WriteString(`{"type":"result","subtype":"success","is_error":false,"result":"# Hi there."}` + "\n")
	case "stream-json-noresult":
		os.Stdout.WriteString(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"only deltas"}}}` + "\n")
	case "stream-json-error":
		os.Stdout.WriteString(`{"type":"result","subtype":"error_max_turns","is_error":true,"result":""}` + "\n")
	case "agy-stream":
		// Echo back the prompt text from the NDJSON user event as an agy-shaped
		// result, so the round trip (envelope in, result out) is exercised.
		in := new(strings.Builder)
		_, _ = io.Copy(in, os.Stdin)
		var evt struct {
			Message struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		text := ""
		if err := json.Unmarshal([]byte(strings.TrimSpace(in.String())), &evt); err == nil && len(evt.Message.Content) > 0 {
			text = evt.Message.Content[0].Text
		}
		os.Stdout.WriteString(`{"event":"init","conversation_id":"x"}` + "\n")
		os.Stdout.WriteString(`{"event":"result","result":{"status":"SUCCESS","response":` + jsonString("GOT:"+text) + `}}` + "\n")
	case "agy-error":
		os.Stdout.WriteString(`{"event":"result","result":{"status":"ERROR","response":"","error":"agy could not do it\nsecond line"}}` + "\n")
	case "acp-agent":
		acpFakeAgent(false)
	case "acp-agent-badversion":
		acpFakeAgent(true)
	case "stream-json-refusal":
		// Observed from claude 2.x: a refusal result, nothing on stderr,
		// and a non-zero exit.
		os.Stdout.WriteString(`{"type":"result","subtype":"success","is_error":false,"result":"","stop_reason":"refusal","terminal_reason":"api_error"}` + "\n")
		os.Exit(1)
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
		s, ok := NewSession("codex", config.Config{}) // codex is a text-mode provider
		if !ok {
			t.Fatal("codex provider should exist")
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
		s, _ := NewSession("codex", config.Config{})
		_, err := s.Generate(context.Background(), Request{Prompt: "x"})
		if err == nil {
			t.Fatal("expected error from failing CLI")
		}
		// Only the first stderr line is surfaced, prefixed with the provider.
		if !strings.Contains(err.Error(), "codex: boom") {
			t.Errorf("error should carry provider + first stderr line, got %q", err)
		}
	})
}

func TestSessionTimeoutAndEmptyStderr(t *testing.T) {
	withExec("default", func() {
		s := &Session{provider: Provider{Name: "test", Bin: "test"}, timeout: 10 * time.Second}
		if _, err := s.Generate(context.Background(), Request{Prompt: "x"}); err != nil {
			t.Fatalf("bounded successful session failed: %v", err)
		}
	})
	withExec("fail-empty", func() {
		s := &Session{provider: Provider{Name: "test", Bin: "test"}}
		if _, err := s.Generate(context.Background(), Request{Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "exit status") {
			t.Fatalf("empty stderr should fall back to the process error, got %v", err)
		}
	})
}

func TestSessionPromptViaStdin(t *testing.T) {
	withExec("echo-stdin", func() {
		// A text-mode provider that delivers the prompt on stdin.
		s := &Session{provider: Provider{Name: "t", Bin: "cli", PromptViaStdin: true}}
		res, err := s.Generate(context.Background(), Request{Prompt: "hello prompt"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(res.Text, "STDIN:hello prompt") {
			t.Errorf("prompt should be delivered on stdin, got %q", res.Text)
		}
	})
}

func TestSessionStreamJSON(t *testing.T) {
	withExec("stream-json", func() {
		s := &Session{provider: Provider{Name: "claude", Bin: "claude", PromptViaStdin: true, StreamJSON: true}}
		var streamed strings.Builder
		res, err := s.Generate(context.Background(), Request{Prompt: "p", OnChunk: func(c string) { streamed.WriteString(c) }})
		if err != nil {
			t.Fatal(err)
		}
		if res.Text != "# Hi there." {
			t.Errorf("stream-json result = %q, want %q", res.Text, "# Hi there.")
		}
		if streamed.String() != "# Hi there." {
			t.Errorf("deltas should stream to OnChunk, got %q", streamed.String())
		}
	})
}

func TestSessionStreamJSONError(t *testing.T) {
	withExec("stream-json-error", func() {
		s := &Session{provider: Provider{Name: "claude", Bin: "claude", PromptViaStdin: true, StreamJSON: true}}
		_, err := s.Generate(context.Background(), Request{Prompt: "p"})
		if err == nil || !strings.Contains(err.Error(), "error_max_turns") {
			t.Errorf("stream-json is_error should surface as an error, got %v", err)
		}
	})
}

// The claude CLI reports a refusal as a result event and then exits 1 with
// an empty stderr. Session used to surface that as "claude: exit status 1",
// indistinguishable from a crash, and the pipeline demoted the provider for
// the rest of the queue.
func TestSessionRefusalOutranksTheExitStatus(t *testing.T) {
	withExec("stream-json-refusal", func() {
		s := &Session{provider: Provider{Name: "claude", Bin: "claude", PromptViaStdin: true, StreamJSON: true}}
		_, err := s.Generate(context.Background(), Request{Prompt: "p"})
		if !errors.Is(err, ErrRefused) {
			t.Fatalf("err = %v, want ErrRefused", err)
		}
		if !strings.HasPrefix(err.Error(), "claude: ") {
			t.Errorf("error should name the provider, got %q", err)
		}
	})
}

func TestSessionPromptViaArg(t *testing.T) {
	withExec("echo-args", func() {
		// amp reads the prompt only as a positional argument (-x, then prompt).
		s, _ := NewSession("amp", config.Config{})
		res, err := s.Generate(context.Background(), Request{Prompt: "arg prompt"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(res.Text, "arg prompt") {
			t.Errorf("prompt should be the final arg, got %q", res.Text)
		}
		if !strings.Contains(res.Text, "-x") {
			t.Errorf("provider args should be present, got %q", res.Text)
		}
	})
}

// Providers confirmed to read stdin must not also leak the prompt into argv,
// where a process listing would expose it along with the source text it quotes.
func TestStdinProvidersKeepThePromptOutOfArgv(t *testing.T) {
	for _, name := range []string{"claude", "codex", "cursor-agent", "goose"} {
		t.Run(name, func(t *testing.T) {
			p, ok := LookupProvider(name)
			if !ok {
				t.Fatalf("provider %q is not registered", name)
			}
			if !p.PromptViaStdin {
				t.Fatalf("%s was verified to read stdin; PromptViaStdin must be set", name)
			}
			withExec("echo-args", func() {
				s, _ := NewSession(name, config.Config{})
				res, err := s.Generate(context.Background(), Request{Prompt: "secret source quote"})
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(res.Text, "secret source quote") {
					t.Errorf("prompt reached argv for %s: %q", name, res.Text)
				}
			})
		})
	}
}

func TestSessionModelFlag(t *testing.T) {
	withExec("echo-args", func() {
		s, _ := NewSession("codex", config.Config{Model: "opus"}) // codex is text-mode with --model
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

func TestSessionStreamJSONNoResult(t *testing.T) {
	withExec("stream-json-noresult", func() {
		s := &Session{provider: Provider{Name: "claude", Bin: "claude", PromptViaStdin: true, StreamJSON: true}}
		res, err := s.Generate(context.Background(), Request{Prompt: "p"})
		if err != nil || res.Text != "only deltas" {
			t.Errorf("no-result stream should return accumulated deltas, got %q %v", res.Text, err)
		}
	})
}

func jsonString(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// agy delivers the prompt as an NDJSON user event on stdin, keeping it out of
// argv, and its answer is the result event's response.
func TestSessionAgyStreamJSON(t *testing.T) {
	withExec("agy-stream", func() {
		s, ok := NewSession("agy", config.Config{})
		if !ok {
			t.Fatal("agy provider should exist")
		}
		res, err := s.Generate(context.Background(), Request{Kind: KindExtract, Prompt: "extract this"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Text != "GOT:extract this" {
			t.Errorf("round trip = %q", res.Text)
		}
	})
	withExec("agy-error", func() {
		s, _ := NewSession("agy", config.Config{})
		_, err := s.Generate(context.Background(), Request{Prompt: "x"})
		if err == nil || !strings.Contains(err.Error(), "could not do it") {
			t.Errorf("an agy ERROR result should surface, got %v", err)
		}
	})
}

func TestAgyUserEventShape(t *testing.T) {
	env := agyUserEvent("hello \"world\"")
	var got struct {
		Event   string `json:"event"`
		Message struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(env), &got); err != nil {
		t.Fatalf("envelope is not valid JSON: %v", err)
	}
	if got.Event != "user" || got.Message.Role != "user" || len(got.Message.Content) != 1 ||
		got.Message.Content[0].Type != "text" || got.Message.Content[0].Text != `hello "world"` {
		t.Errorf("envelope = %s", env)
	}
	if !strings.HasSuffix(env, "\n") {
		t.Error("the envelope must be newline-terminated for NDJSON")
	}
}

func TestParseAgyStreamJSONStreamsAndErrors(t *testing.T) {
	stream := strings.Join([]string{
		`{"event":"init"}`,
		`not json, a banner`,
		`{"event":"assistant","content":{"text":"par"}}`,
		`{"event":"assistant","content":{"text":"tial"}}`,
		`{"event":"result","result":{"status":"SUCCESS","response":"final answer"}}`,
	}, "\n")
	var streamed strings.Builder
	out, err := parseAgyStreamJSON(strings.NewReader(stream), func(s string) { streamed.WriteString(s) })
	if err != nil || out != "final answer" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if streamed.String() != "partial" {
		t.Errorf("chunks = %q", streamed.String())
	}
	// No result event: fall back to the accumulated text.
	out, _ = parseAgyStreamJSON(strings.NewReader(`{"event":"assistant","content":{"text":"only chunks"}}`), nil)
	if out != "only chunks" {
		t.Errorf("fallback = %q", out)
	}
	// An error result with no message uses the default.
	_, err = parseAgyStreamJSON(strings.NewReader(`{"event":"result","result":{"status":"ERROR"}}`), nil)
	if err == nil || !strings.Contains(err.Error(), "agy reported an error") {
		t.Errorf("default error = %v", err)
	}
}
