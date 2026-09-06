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
	"sync"
	"time"

	"github.com/sebastienrousseau/draft/config"
)

// ACP drives an agent over the Agent Client Protocol: JSON-RPC 2.0 on the
// agent's stdio, one long-lived process for the life of the engine, and one
// fresh session per Generate call.
//
// One process, many sessions, is the shape that matters. A session carries
// conversation state, and claim extraction needs the opposite: every section
// must be read on its own, or a fact from section 3 leaks into the claims for
// section 9 and the ledger stops being grounded in the section it names. So
// the process is reused and the session never is.
//
// The protocol is also the first place a declined prompt is a first-class
// outcome rather than an exit status: session/prompt answers with a stopReason,
// and "refusal" maps straight onto ErrRefused.
//
// What ACP does not buy is wall clock. Measured against claude-code-acp 0.16,
// a warm second session costs about what a cold `claude -p` costs, because
// the adapter starts an agent per session underneath. The win is a standard
// transport with typed outcomes, not a faster one.
type ACP struct {
	provider Provider
	model    string
	timeout  time.Duration

	mu   sync.Mutex // guards conn across Generate calls
	conn *acpConn
}

// NewACP builds an ACP engine for a provider registered with ACP set. It
// returns false if the name is unknown or the provider is not an ACP agent.
func NewACP(name string, cfg config.Config) (*ACP, bool) {
	p, ok := LookupProvider(name)
	if !ok || !p.ACP {
		return nil, false
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = p.DefaultModel
	}
	return &ACP{provider: p, model: model, timeout: cfg.CallTimeout}, true
}

// Name implements Engine.
func (a *ACP) Name() string { return a.provider.Name }

// Generate implements Engine: a new session on the shared agent process, one
// prompt, the streamed answer, and the stop reason turned into a Result.
func (a *ACP) Generate(ctx context.Context, req Request) (Result, error) {
	if a.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.timeout)
		defer cancel()
	}
	conn, err := a.connect(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("%s: %w", a.provider.Name, err)
	}

	// The session's working directory is a fresh empty one, for the same
	// reason Session runs in one: an agent reads whatever configuration it
	// finds there, and the prompt carries verbatim third-party text.
	dir, err := mkTempDir("", "draft-acp-session-")
	if err != nil {
		return Result{}, fmt.Errorf("%s: could not create an isolated working directory: %w", a.provider.Name, err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	var created struct {
		SessionID string `json:"sessionId"`
	}
	if err := conn.call(ctx, "session/new", map[string]any{"cwd": dir, "mcpServers": []any{}}, &created); err != nil {
		return Result{}, fmt.Errorf("%s: session/new: %w", a.provider.Name, err)
	}
	if a.model != "" && a.model != "default" {
		// A model the agent does not know is a configuration error worth
		// surfacing; an agent that lacks the method altogether is not.
		err := conn.call(ctx, "session/set_model", map[string]any{"sessionId": created.SessionID, "modelId": a.model}, nil)
		var rpcErr *acpError
		if err != nil && (!errors.As(err, &rpcErr) || rpcErr.Code != acpMethodNotFound) {
			return Result{}, fmt.Errorf("%s: session/set_model %q: %w", a.provider.Name, a.model, err)
		}
	}

	sess := conn.openSession(created.SessionID, req.OnChunk)
	defer conn.closeSession(created.SessionID)

	var turn struct {
		StopReason string `json:"stopReason"`
	}
	params := map[string]any{
		"sessionId": created.SessionID,
		"prompt":    []map[string]any{{"type": "text", "text": req.Prompt}},
	}
	if err := conn.call(ctx, "session/prompt", params, &turn); err != nil {
		if ctx.Err() != nil {
			// Tell the agent to stop spending on a turn nobody will read.
			conn.notify("session/cancel", map[string]any{"sessionId": created.SessionID})
			return Result{}, ctx.Err()
		}
		return Result{}, fmt.Errorf("%s: session/prompt: %w", a.provider.Name, err)
	}

	text := sess.text()
	switch strings.ToLower(strings.TrimSpace(turn.StopReason)) {
	case "refusal":
		return Result{}, fmt.Errorf("%s: %w", a.provider.Name, ErrRefused)
	case "cancelled":
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		return Result{}, fmt.Errorf("%s: the agent cancelled the turn", a.provider.Name)
	case "max_tokens", "max_turn_requests":
		return Result{Text: strings.TrimSpace(text), Truncated: true}, nil
	default:
		return Result{Text: strings.TrimSpace(text)}, nil
	}
}

// connect returns the live agent process, starting one if none is running or
// the last one has exited. A dead process is replaced once per call, not
// retried in a loop: if the agent cannot stay up the caller should hear so.
func (a *ACP) connect(ctx context.Context) (*acpConn, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn != nil && a.conn.alive() {
		return a.conn, nil
	}
	if a.conn != nil {
		a.conn.close()
		a.conn = nil
	}
	conn, err := startACP(ctx, a.provider)
	if err != nil {
		return nil, err
	}
	a.conn = conn
	return conn, nil
}

// Close stops the agent process. The engine can be used again afterwards; the
// next Generate starts a fresh one.
func (a *ACP) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn == nil {
		return nil
	}
	a.conn.close()
	a.conn = nil
	return nil
}

// acpProtocolVersion is the major protocol version this client speaks.
const acpProtocolVersion = 1

// acpMethodNotFound is the JSON-RPC code for an unknown method.
const acpMethodNotFound = -32601

// acpError is a JSON-RPC error object.
type acpError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *acpError) Error() string {
	if len(e.Data) > 0 {
		return fmt.Sprintf("%s (%d): %s", e.Message, e.Code, string(e.Data))
	}
	return fmt.Sprintf("%s (%d)", e.Message, e.Code)
}

// acpMessage is any JSON-RPC frame on the wire. Requests have a method and an
// id, notifications a method and no id, responses an id and a result or error.
type acpMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *acpError       `json:"error,omitempty"`
}

// acpSession collects one session's streamed text. Chunks are appended by the
// reader goroutine as they arrive, so the collected text is complete by the
// time the prompt's response is delivered: the agent sends its updates before
// it answers the request.
type acpSession struct {
	mu      sync.Mutex
	buf     strings.Builder
	onChunk func(string)
}

func (s *acpSession) add(chunk string) {
	s.mu.Lock()
	s.buf.WriteString(chunk)
	s.mu.Unlock()
	if s.onChunk != nil && chunk != "" {
		s.onChunk(chunk)
	}
}

func (s *acpSession) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// acpConn is one agent process and the JSON-RPC plumbing over its stdio.
type acpConn struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	dir   string

	wmu sync.Mutex // serialises writes to stdin

	mu       sync.Mutex
	nextID   int64
	pending  map[int64]chan acpMessage
	sessions map[string]*acpSession
	closed   bool
	exitErr  error
	done     chan struct{}
	stderr   *limitedBuffer
}

// startACP launches the agent, wires the reader, and performs the initialize
// handshake. The process runs in an empty directory of its own with draft's
// configuration removed from its environment, exactly like a one-shot Session.
func startACP(ctx context.Context, p Provider) (*acpConn, error) {
	dir, err := mkTempDir("", "draft-acp-")
	if err != nil {
		return nil, fmt.Errorf("could not create an isolated working directory: %w", err)
	}
	// The process must outlive any single call's context: it is shared by
	// every Generate on the engine. Cancellation of a call is expressed to
	// the agent as session/cancel, never by killing the process.
	cmd := execCommand(context.Background(), p.Bin, p.Args...)
	cmd.Dir = dir
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = sessionEnv(cmd.Env)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	stderr := &limitedBuffer{limit: 8 * 1024}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	c := &acpConn{
		cmd:      cmd,
		stdin:    stdin,
		dir:      dir,
		pending:  map[int64]chan acpMessage{},
		sessions: map[string]*acpSession{},
		done:     make(chan struct{}),
		stderr:   stderr,
	}
	go c.read(stdout)

	var init struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	params := map[string]any{
		"protocolVersion": acpProtocolVersion,
		// draft never lets an agent touch the file system; saying so up
		// front keeps the agent from asking.
		"clientCapabilities": map[string]any{"fs": map[string]any{"readTextFile": false, "writeTextFile": false}},
	}
	if err := c.call(ctx, "initialize", params, &init); err != nil {
		c.close()
		return nil, fmt.Errorf("initialize: %w", err)
	}
	if init.ProtocolVersion != acpProtocolVersion {
		c.close()
		return nil, fmt.Errorf("initialize: agent speaks protocol version %d, this client speaks %d", init.ProtocolVersion, acpProtocolVersion)
	}
	return c, nil
}

// read is the single reader of the agent's stdout. It routes responses to
// their callers, streams session updates into the open session, and answers
// the agent's own requests — always with a refusal, because draft grants no
// tools and never will.
func (c *acpConn) read(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var msg acpMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue // not a frame; agents log to stdout by mistake sometimes
		}
		switch {
		case msg.Method != "" && len(msg.ID) > 0:
			c.answerRequest(msg)
		case msg.Method != "":
			c.handleNotification(msg)
		default:
			var id int64
			if err := json.Unmarshal(msg.ID, &id); err != nil {
				continue
			}
			c.mu.Lock()
			ch, ok := c.pending[id]
			delete(c.pending, id)
			c.mu.Unlock()
			if ok {
				ch <- msg
			}
		}
	}
	// EOF or a read error: the process is gone. Fail every waiter so no call
	// hangs on a reply that will never come.
	err := scanner.Err()
	if err == nil {
		err = errors.New("the agent closed its output")
	}
	if waitErr := c.cmd.Wait(); waitErr != nil {
		err = fmt.Errorf("%w (%v)", err, waitErr)
	}
	if msg := strings.TrimSpace(c.stderr.String()); msg != "" {
		err = fmt.Errorf("%w: %s", err, firstLine(msg))
	}
	c.mu.Lock()
	c.exitErr = err
	for id, ch := range c.pending {
		delete(c.pending, id)
		ch <- acpMessage{Error: &acpError{Code: -32000, Message: err.Error()}}
	}
	c.mu.Unlock()
	close(c.done)
}

// answerRequest replies to a request the agent makes of the client. Permission
// requests are cancelled and everything else is unsupported: the agent gets
// text in and text out, nothing more.
func (c *acpConn) answerRequest(msg acpMessage) {
	var reply map[string]any
	if msg.Method == "session/request_permission" {
		reply = map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}}
	} else {
		reply = map[string]any{"jsonrpc": "2.0", "id": msg.ID, "error": map[string]any{"code": acpMethodNotFound, "message": "not supported by draft"}}
	}
	_ = c.write(reply)
}

// handleNotification streams agent_message_chunk updates into their session.
// Every other update kind (plans, tool calls, available commands) is
// irrelevant to a text-only client and dropped.
func (c *acpConn) handleNotification(msg acpMessage) {
	if msg.Method != "session/update" {
		return
	}
	var params struct {
		SessionID string `json:"sessionId"`
		Update    struct {
			Kind    string `json:"sessionUpdate"`
			Content struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"update"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}
	if params.Update.Kind != "agent_message_chunk" || params.Update.Content.Type != "text" {
		return
	}
	c.mu.Lock()
	sess := c.sessions[params.SessionID]
	c.mu.Unlock()
	if sess != nil {
		sess.add(params.Update.Content.Text)
	}
}

func (c *acpConn) openSession(id string, onChunk func(string)) *acpSession {
	s := &acpSession{onChunk: onChunk}
	c.mu.Lock()
	c.sessions[id] = s
	c.mu.Unlock()
	return s
}

func (c *acpConn) closeSession(id string) {
	c.mu.Lock()
	delete(c.sessions, id)
	c.mu.Unlock()
}

// call sends a request and waits for its response, decoding the result into
// out when out is non-nil.
func (c *acpConn) call(ctx context.Context, method string, params any, out any) error {
	c.mu.Lock()
	if c.exitErr != nil {
		err := c.exitErr
		c.mu.Unlock()
		return err
	}
	c.nextID++
	id := c.nextID
	ch := make(chan acpMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return err
	}
	select {
	case msg := <-ch:
		if msg.Error != nil {
			return msg.Error
		}
		if out != nil && len(msg.Result) > 0 {
			return json.Unmarshal(msg.Result, out)
		}
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	}
}

// notify sends a notification: a request without an id, which gets no reply.
func (c *acpConn) notify(method string, params any) {
	_ = c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *acpConn) write(frame any) error {
	b, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err = c.stdin.Write(append(b, '\n'))
	return err
}

func (c *acpConn) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

// close stops the agent and removes its directory. It is safe to call twice.
func (c *acpConn) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	<-c.done
	_ = os.RemoveAll(c.dir)
}

// limitedBuffer keeps the first limit bytes written to it: enough of an
// agent's stderr to explain a failure, never enough to exhaust memory.
type limitedBuffer struct {
	mu    sync.Mutex
	buf   strings.Builder
	limit int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if room := l.limit - l.buf.Len(); room > 0 {
		if len(p) > room {
			l.buf.Write(p[:room])
		} else {
			l.buf.Write(p)
		}
	}
	return len(p), nil
}

func (l *limitedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}
