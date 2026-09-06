// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/sebastienrousseau/draft/config"
)

// acpFakeAgent is the helper-process side of the ACP tests: a minimal agent
// that speaks enough of the protocol to exercise every branch of the client.
// It runs in the test binary under HELPER_MODE=acp-agent and exits when stdin
// closes. Behaviour is steered by words in the prompt so one agent serves
// every test.
func acpFakeAgent(badVersion bool) {
	newFails := os.Getenv("HELPER_ACP_NEWFAIL") == "1"
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	sessions := 0
	send := func(v any) {
		b, _ := json.Marshal(v)
		os.Stdout.Write(append(b, '\n'))
	}
	reply := func(id json.RawMessage, result any) {
		send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	}
	update := func(sid, kind string, content any) {
		send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
			"sessionId": sid, "update": map[string]any{"sessionUpdate": kind, "content": content},
		}})
	}
	chunk := func(sid, text string) {
		update(sid, "agent_message_chunk", map[string]any{"type": "text", "text": text})
	}
	for in.Scan() {
		var msg acpMessage
		if err := json.Unmarshal(in.Bytes(), &msg); err != nil {
			continue
		}
		switch msg.Method {
		case "initialize":
			v := acpProtocolVersion
			if badVersion {
				v = acpProtocolVersion + 1
			}
			// A stray log line and a non-JSON line: the client must skip both.
			os.Stdout.WriteString("agent starting\n")
			reply(msg.ID, map[string]any{"protocolVersion": v, "agentCapabilities": map[string]any{}})
		case "session/new":
			if newFails {
				send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "error": map[string]any{"code": -32603, "message": "Internal error", "data": map[string]any{"details": "Query closed before response received"}}})
				continue
			}
			sessions++
			reply(msg.ID, map[string]any{"sessionId": fmt.Sprintf("s%d", sessions)})
		case "session/set_model":
			var p struct {
				ModelID string `json:"modelId"`
			}
			_ = json.Unmarshal(msg.Params, &p)
			if p.ModelID == "bogus" {
				send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "error": map[string]any{"code": -32602, "message": "unknown model"}})
			} else {
				reply(msg.ID, map[string]any{})
			}
		case "session/cancel":
			// Nothing to do: the slow turn below checks for it by timing.
		case "session/prompt":
			var p struct {
				SessionID string `json:"sessionId"`
				Prompt    []struct {
					Text string `json:"text"`
				} `json:"prompt"`
			}
			_ = json.Unmarshal(msg.Params, &p)
			text := ""
			if len(p.Prompt) > 0 {
				text = p.Prompt[0].Text
			}
			update(p.SessionID, "available_commands_update", nil)
			switch {
			case strings.Contains(text, "REFUSE"):
				reply(msg.ID, map[string]any{"stopReason": "refusal"})
			case strings.Contains(text, "TRUNCATE"):
				chunk(p.SessionID, "half an ")
				reply(msg.ID, map[string]any{"stopReason": "max_tokens"})
			case strings.Contains(text, "DIE"):
				os.Stderr.WriteString("agent crashed: out of cheese\n")
				os.Exit(3)
			case strings.Contains(text, "CANCELLED"):
				reply(msg.ID, map[string]any{"stopReason": "cancelled"})
			case strings.Contains(text, "SLOW"):
				time.Sleep(2 * time.Second)
				reply(msg.ID, map[string]any{"stopReason": "end_turn"})
			case strings.Contains(text, "PERMISSION"):
				// Ask for a tool, then report what the client decided.
				send(map[string]any{"jsonrpc": "2.0", "id": 1000, "method": "session/request_permission", "params": map[string]any{"sessionId": p.SessionID}})
				outcome := "none"
				for in.Scan() {
					var r struct {
						ID     json.RawMessage `json:"id"`
						Result struct {
							Outcome struct {
								Outcome string `json:"outcome"`
							} `json:"outcome"`
						} `json:"result"`
					}
					if json.Unmarshal(in.Bytes(), &r) == nil && string(r.ID) == "1000" {
						outcome = r.Result.Outcome.Outcome
						break
					}
				}
				// And something the client does not support at all.
				send(map[string]any{"jsonrpc": "2.0", "id": 1001, "method": "fs/read_text_file", "params": map[string]any{}})
				for in.Scan() {
					var r struct {
						ID    json.RawMessage `json:"id"`
						Error *acpError       `json:"error"`
					}
					if json.Unmarshal(in.Bytes(), &r) == nil && string(r.ID) == "1001" {
						if r.Error != nil {
							outcome += fmt.Sprintf(",fs:%d", r.Error.Code)
						}
						break
					}
				}
				chunk(p.SessionID, "permission:"+outcome)
				reply(msg.ID, map[string]any{"stopReason": "end_turn"})
			default:
				chunk(p.SessionID, "Hello from ")
				chunk(p.SessionID, p.SessionID)
				reply(msg.ID, map[string]any{"stopReason": "end_turn"})
			}
		}
	}
	os.Exit(0)
}

func acpEngine(t *testing.T, cfg config.Config) *ACP {
	t.Helper()
	a, ok := NewACP("claude-acp", cfg)
	if !ok {
		t.Fatal("claude-acp should be a registered ACP provider")
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func TestACPStreamsTextAndReusesTheProcess(t *testing.T) {
	withExec("acp-agent", func() {
		a := acpEngine(t, config.Config{})
		var streamed strings.Builder
		res, err := a.Generate(context.Background(), Request{Prompt: "first", OnChunk: func(s string) { streamed.WriteString(s) }})
		if err != nil {
			t.Fatal(err)
		}
		if res.Text != "Hello from s1" || streamed.String() != "Hello from s1" {
			t.Errorf("first call = %q streamed %q", res.Text, streamed.String())
		}
		// The second call must get a new session on the same process: the
		// fake numbers sessions per process, so a restart would say s1 again.
		res, err = a.Generate(context.Background(), Request{Prompt: "second"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Text != "Hello from s2" {
			t.Errorf("second call = %q, want a fresh session on the same agent", res.Text)
		}
		if res.Truncated {
			t.Error("an end_turn stop must not be reported as truncated")
		}
	})
}

func TestACPStopReasons(t *testing.T) {
	withExec("acp-agent", func() {
		a := acpEngine(t, config.Config{})
		_, err := a.Generate(context.Background(), Request{Prompt: "please REFUSE"})
		if !errors.Is(err, ErrRefused) {
			t.Fatalf("refusal stopReason: err = %v, want ErrRefused", err)
		}
		if !strings.HasPrefix(err.Error(), "claude-acp: ") {
			t.Errorf("error should name the provider, got %q", err)
		}
		res, err := a.Generate(context.Background(), Request{Prompt: "TRUNCATE"})
		if err != nil || !res.Truncated || res.Text != "half an" {
			t.Errorf("max_tokens stop: got (%q, %v, %v)", res.Text, res.Truncated, err)
		}
	})
}

// The agent may ask the client for a tool or a file. draft never grants either:
// a permission request is cancelled and anything else is unsupported.
func TestACPDeclinesAgentRequests(t *testing.T) {
	withExec("acp-agent", func() {
		a := acpEngine(t, config.Config{})
		res, err := a.Generate(context.Background(), Request{Prompt: "PERMISSION"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Text != fmt.Sprintf("permission:cancelled,fs:%d", acpMethodNotFound) {
			t.Errorf("agent saw %q", res.Text)
		}
	})
}

func TestACPRestartsAfterTheAgentDies(t *testing.T) {
	withExec("acp-agent", func() {
		a := acpEngine(t, config.Config{})
		_, err := a.Generate(context.Background(), Request{Prompt: "DIE"})
		if err == nil {
			t.Fatal("a dead agent must surface as an error")
		}
		if !strings.Contains(err.Error(), "out of cheese") {
			t.Errorf("the agent's stderr should explain the death, got %q", err)
		}
		res, err := a.Generate(context.Background(), Request{Prompt: "again"})
		if err != nil {
			t.Fatalf("the next call should start a fresh agent: %v", err)
		}
		if res.Text != "Hello from s1" {
			t.Errorf("after a restart the session count starts over; got %q", res.Text)
		}
	})
}

func TestACPModelSelection(t *testing.T) {
	withExec("acp-agent", func() {
		ok := acpEngine(t, config.Config{Model: "opus"})
		if _, err := ok.Generate(context.Background(), Request{Prompt: "x"}); err != nil {
			t.Errorf("a known model should be accepted: %v", err)
		}
		bad := acpEngine(t, config.Config{Model: "bogus"})
		_, err := bad.Generate(context.Background(), Request{Prompt: "x"})
		if err == nil || !strings.Contains(err.Error(), "session/set_model") {
			t.Errorf("an unknown model should fail the call, got %v", err)
		}
	})
}

func TestACPCancellationStopsTheWait(t *testing.T) {
	withExec("acp-agent", func() {
		a := acpEngine(t, config.Config{})
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := a.Generate(ctx, Request{Prompt: "SLOW"})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want the context's deadline", err)
		}
		if time.Since(start) > time.Second {
			t.Error("cancellation waited for the agent instead of returning")
		}
		// The engine's own call timeout behaves the same way.
		timed := acpEngine(t, config.Config{CallTimeout: 150 * time.Millisecond})
		if _, err := timed.Generate(context.Background(), Request{Prompt: "SLOW"}); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("CallTimeout: err = %v", err)
		}
	})
}

// The adapter answers session/new with an error when the agent underneath
// cannot start (seen when a nested Claude Code session's environment leaks
// in). That is a call failure with the agent's own explanation, not a hang.
func TestACPSessionCreationFailure(t *testing.T) {
	t.Setenv("HELPER_ACP_NEWFAIL", "1")
	withExec("acp-agent", func() {
		a := acpEngine(t, config.Config{})
		_, err := a.Generate(context.Background(), Request{Prompt: "x"})
		if err == nil || !strings.Contains(err.Error(), "session/new") || !strings.Contains(err.Error(), "Query closed") {
			t.Errorf("err = %v, want the agent's session/new error", err)
		}
	})
}

// An agent that reports a cancelled turn the client never asked for is an
// error, not an empty article.
func TestACPUnsolicitedCancelIsAnError(t *testing.T) {
	withExec("acp-agent", func() {
		a := acpEngine(t, config.Config{})
		_, err := a.Generate(context.Background(), Request{Prompt: "CANCELLED"})
		if err == nil || !strings.Contains(err.Error(), "cancelled the turn") {
			t.Errorf("err = %v", err)
		}
	})
}

func TestACPProtocolVersionMismatch(t *testing.T) {
	withExec("acp-agent-badversion", func() {
		a := acpEngine(t, config.Config{})
		_, err := a.Generate(context.Background(), Request{Prompt: "x"})
		if err == nil || !strings.Contains(err.Error(), "protocol version") {
			t.Errorf("a version mismatch must be refused at initialize, got %v", err)
		}
	})
}

func TestACPStartFailures(t *testing.T) {
	withExec("acp-agent", func() {
		orig := mkTempDir
		mkTempDir = func(string, string) (string, error) { return "", errors.New("disk full") }
		defer func() { mkTempDir = orig }()
		a := acpEngine(t, config.Config{})
		if _, err := a.Generate(context.Background(), Request{Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "disk full") {
			t.Errorf("temp dir failure should surface, got %v", err)
		}
	})
	withExec("fail-empty", func() {
		// An agent that exits before answering initialize.
		a := acpEngine(t, config.Config{})
		if _, err := a.Generate(context.Background(), Request{Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "initialize") {
			t.Errorf("an agent that dies during the handshake should fail initialize, got %v", err)
		}
	})
	a := &ACP{provider: Provider{Name: "none", Bin: "definitely-not-a-binary-on-path-1234"}}
	if _, err := a.Generate(context.Background(), Request{Prompt: "x"}); err == nil {
		t.Error("a missing binary must be an error")
	}
	if err := a.Close(); err != nil {
		t.Errorf("Close on an unstarted engine: %v", err)
	}
}

func TestNewEngineDispatchesByTransport(t *testing.T) {
	if e, ok := NewEngine("claude-acp", config.Config{}); !ok {
		t.Fatal("claude-acp should resolve")
	} else if _, isACP := e.(*ACP); !isACP {
		t.Errorf("claude-acp resolved to %T, want *ACP", e)
	}
	if e, ok := NewEngine("claude", config.Config{}); !ok {
		t.Fatal("claude should resolve")
	} else if _, isSession := e.(*Session); !isSession {
		t.Errorf("claude resolved to %T, want *Session", e)
	}
	if _, ok := NewEngine("no-such-provider", config.Config{}); ok {
		t.Error("an unknown provider must not resolve")
	}
	if _, ok := NewACP("claude", config.Config{}); ok {
		t.Error("NewACP must refuse a one-shot provider")
	}
	if got := ResolveModel(config.Config{}, &ACP{provider: Provider{Name: "claude-acp"}}); got != "sonnet" {
		t.Errorf("claude-acp default model = %q", got)
	}
}

func TestLimitedBufferKeepsTheHead(t *testing.T) {
	var b limitedBuffer
	b.limit = 5
	if n, _ := b.Write([]byte("abc")); n != 3 {
		t.Fatal("Write must report the full length")
	}
	_, _ = b.Write([]byte("defgh"))
	_, _ = b.Write([]byte("ijk"))
	if b.String() != "abcde" {
		t.Errorf("buffer = %q, want the first five bytes", b.String())
	}
}

// Wire-level branches the fake agent cannot provoke from the outside.
func TestACPConnInternals(t *testing.T) {
	e := &acpError{Code: -32603, Message: "Internal error", Data: json.RawMessage(`{"details":"boom"}`)}
	if got := e.Error(); got != `Internal error (-32603): {"details":"boom"}` {
		t.Errorf("Error() with data = %q", got)
	}
	if got := (&acpError{Code: 1, Message: "m"}).Error(); got != "m (1)" {
		t.Errorf("Error() without data = %q", got)
	}

	// A call on a connection whose agent has already exited fails at once
	// with the exit error, without writing anything.
	dead := &acpConn{pending: map[int64]chan acpMessage{}, sessions: map[string]*acpSession{}, exitErr: errors.New("gone")}
	if err := dead.call(context.Background(), "x", nil, nil); err == nil || err.Error() != "gone" {
		t.Errorf("call on a dead agent = %v", err)
	}

	// Frames that do not concern anyone are dropped, not crashed on.
	c := &acpConn{pending: map[int64]chan acpMessage{}, sessions: map[string]*acpSession{}}
	c.handleNotification(acpMessage{Method: "session/update", Params: json.RawMessage(`not json`)})
	c.handleNotification(acpMessage{Method: "session/update", Params: json.RawMessage(`{"sessionId":"nobody","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"x"}}}`)})
	c.handleNotification(acpMessage{Method: "other"})
	sess := c.openSession("s1", nil)
	c.handleNotification(acpMessage{Method: "session/update", Params: json.RawMessage(`{"sessionId":"s1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"hi"}}}`)})
	if sess.text() != "hi" {
		t.Errorf("session text = %q", sess.text())
	}
	c.closeSession("s1")

	// A response with an id nobody is waiting for, or that is not a number,
	// is ignored by the reader.
	pr, pw := io.Pipe()
	c.cmd = exec.Command("true")
	c.stderr = &limitedBuffer{limit: 10}
	c.done = make(chan struct{})
	if err := c.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go c.read(pr)
	_, _ = pw.Write([]byte(`{"jsonrpc":"2.0","id":"abc","result":{}}` + "\n"))
	_, _ = pw.Write([]byte(`{"jsonrpc":"2.0","id":99,"result":{}}` + "\n"))
	_ = pw.Close()
	<-c.done
	if c.exitErr == nil {
		t.Error("EOF must record an exit error")
	}
	// close is idempotent, including after the reader has already finished.
	c.stdin = closedWriter{}
	c.close()
	c.close()

	// A write to a closed stdin surfaces from call.
	wr := &acpConn{pending: map[int64]chan acpMessage{}, sessions: map[string]*acpSession{}, stdin: closedWriter{}}
	if err := wr.call(context.Background(), "x", nil, nil); err == nil {
		t.Error("call must report a failed write")
	}
	if got := wr.write(func() {}); got == nil {
		t.Error("an unmarshalable frame must be an error")
	}
}

type closedWriter struct{}

func (closedWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }
func (closedWriter) Close() error              { return nil }
