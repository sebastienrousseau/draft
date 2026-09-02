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
	"time"

	"github.com/sebastienrousseau/draft/config"
)

// execCommand builds the command used to invoke a provider CLI. It is a package
// variable so tests can substitute a fake command without spawning processes.
var execCommand = exec.CommandContext

// mkTempDir is os.MkdirTemp, as a variable so the failure path is testable.
var mkTempDir = os.MkdirTemp

// sessionEnv is the environment handed to a provider subprocess.
//
// draft's own configuration is removed: DRAFT_* names steer draft, and letting
// them reach an agent that reads its own environment lets one tool's settings
// silently redirect another's. Everything else is passed through, deliberately.
// A tight allowlist reads as the safer choice and is not: every provider finds
// its credentials somewhere different in the environment, so a list that is
// wrong breaks authentication and the failure surfaces as a confusing provider
// error rather than as a security decision. The isolation that actually bounds
// the blast radius is the empty working directory and the absence of tool
// grants, not the variable list.
func sessionEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		if strings.HasPrefix(kv, "DRAFT_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// Session generates text by driving a token-free agent CLI (Claude, Codex,
// Gemini, Copilot, Cursor, Amp, Crush, Goose, Grok, Qwen, ...) in headless
// mode. Authentication comes entirely from the CLI's own logged-in session.
type Session struct {
	provider Provider
	model    string
	timeout  time.Duration
}

// NewSession builds a Session for a named provider, applying the configured
// model override (falling back to the provider's default). It returns false if
// the provider name is unknown.
func NewSession(name string, cfg config.Config) (*Session, bool) {
	p, ok := LookupProvider(name)
	if !ok {
		return nil, false
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = p.DefaultModel
	}
	return &Session{provider: p, model: model, timeout: cfg.CallTimeout}, true
}

// Name implements Engine, returning the provider name.
func (s *Session) Name() string { return s.provider.Name }

// Generate runs the provider CLI once and returns its assistant text, streaming
// output to req.OnChunk as it arrives. The large grounded prompt is delivered
// on stdin when the provider supports it (avoiding ARG_MAX limits) and as a
// positional argument otherwise.
func (s *Session) Generate(ctx context.Context, req Request) (Result, error) {
	// Bound the call so a wedged provider CLI cannot hang the run forever.
	// The ceiling is generous (see config.DefaultCallTimeout); it exists to
	// catch a hang, not to cut short a slow but healthy generation.
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	// Run the provider in an empty directory of our own, never the user's.
	//
	// A provider CLI started in the working directory loads whatever agent
	// configuration lives there — CLAUDE.md, AGENTS.md, .mcp.json, project
	// settings — and draft's prompts carry verbatim text from a third-party
	// document. Inheriting that context both widens the blast radius of a
	// crafted source and pays for MCP server startup on every one of the
	// dozen-plus calls a paper costs.
	dir, err := mkTempDir("", "draft-session-")
	if err != nil {
		return Result{}, fmt.Errorf("%s: could not create an isolated working directory: %w", s.provider.Name, err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	args := append([]string{}, s.provider.Args...)
	if s.provider.ModelFlag != "" && s.model != "" && s.model != "default" {
		args = append(args, s.provider.ModelFlag, s.model)
	}

	cmd := execCommand(ctx, s.provider.Bin, args...)
	cmd.Dir = dir
	// Filter whatever environment the command already carries, rather than
	// replacing it: exec.CommandContext leaves Env nil to mean "inherit", but
	// a caller that has set one deliberately must keep it.
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = sessionEnv(cmd.Env)
	if s.provider.PromptViaStdin {
		cmd.Stdin = strings.NewReader(req.Prompt)
		if s.provider.StdinPlaceholder != "" {
			cmd.Args = append(cmd.Args, s.provider.StdinPlaceholder)
		}
	} else {
		cmd.Args = append(cmd.Args, req.Prompt)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return Result{}, err
	}

	var out string
	var truncated bool
	var streamErr error
	if s.provider.StreamJSON {
		out, truncated, streamErr = parseStreamJSON(stdout, req.OnChunk)
	} else {
		out, streamErr = streamAll(stdout, req.OnChunk)
	}

	if err := cmd.Wait(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return Result{}, fmt.Errorf("%s: %s", s.provider.Name, firstLine(msg))
	}
	if streamErr != nil {
		return Result{}, fmt.Errorf("%s: %s", s.provider.Name, firstLine(streamErr.Error()))
	}
	return Result{Text: strings.TrimSpace(out), Truncated: truncated}, nil
}

// parseStreamJSON reads the Claude Code stream-json event stream, forwarding
// each text delta to onChunk as it arrives (for a live preview) and returning
// the complete assistant text. The authoritative final `result` is preferred
// over the accumulated deltas; an error result is surfaced as an error.
//
// It also reports whether the model stopped on a length limit, read from the
// message_delta event's stop_reason. Without that the pipeline's continuation
// machinery could never fire for a session provider, and a length-limited stop
// would surface much later as a "article appears truncated" rule violation
// costing a full rewrite.
func parseStreamJSON(r io.Reader, onChunk func(string)) (text string, truncated bool, err error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var acc strings.Builder
	var result string
	var haveResult, isError bool
	for scanner.Scan() {
		var ev struct {
			Type  string `json:"type"`
			Event struct {
				Type  string `json:"type"`
				Delta struct {
					Type       string `json:"type"`
					Text       string `json:"text"`
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
			} `json:"event"`
			Subtype string `json:"subtype"`
			IsError bool   `json:"is_error"`
			Result  string `json:"result"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue // ignore non-JSON or partial lines
		}
		switch ev.Type {
		case "stream_event":
			switch ev.Event.Type {
			case "content_block_delta":
				if ev.Event.Delta.Type == "text_delta" && ev.Event.Delta.Text != "" {
					acc.WriteString(ev.Event.Delta.Text)
					if onChunk != nil {
						onChunk(ev.Event.Delta.Text)
					}
				}
			case "message_delta":
				if isLengthStop(ev.Event.Delta.StopReason) {
					truncated = true
				}
			}
		case "result":
			result = ev.Result
			haveResult = true
			isError = ev.IsError
			if isError && result == "" {
				result = ev.Subtype
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return acc.String(), truncated, err
	}
	if isError {
		if result == "" {
			result = "provider reported an error"
		}
		return "", truncated, fmt.Errorf("%s", result)
	}
	if haveResult && strings.TrimSpace(result) != "" {
		return result, truncated, nil
	}
	return acc.String(), truncated, nil
}

// isLengthStop reports whether a stop reason means the model ran out of room
// rather than finishing. Providers spell it differently; match all the forms
// in use so a new one degrades to "not truncated" rather than misreporting.
func isLengthStop(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "max_tokens", "max_output_tokens", "length":
		return true
	}
	return false
}

// streamAll reads r to completion, forwarding each chunk to onChunk if set,
// and returns the accumulated text. A read error other than EOF is returned
// rather than swallowed: silently treating a broken pipe as a complete
// response is how a half-written article reaches the validator looking whole.
func streamAll(r io.Reader, onChunk func(string)) (string, error) {
	var out strings.Builder
	buf := make([]byte, 4096)
	reader := bufio.NewReader(r)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			out.WriteString(chunk)
			if onChunk != nil {
				onChunk(chunk)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return out.String(), nil
			}
			return out.String(), readErr
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
