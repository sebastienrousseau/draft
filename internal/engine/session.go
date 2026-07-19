// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/sebastienrousseau/draft/internal/config"
)

// execCommand builds the command used to invoke a provider CLI. It is a package
// variable so tests can substitute a fake command without spawning processes.
var execCommand = exec.CommandContext

// Session generates text by driving a token-free agent CLI (Claude, Codex,
// Gemini, Copilot, Cursor, Amp, Crush, Goose, Grok, Qwen, ...) in headless
// mode. Authentication comes entirely from the CLI's own logged-in session.
type Session struct {
	provider Provider
	model    string
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
	return &Session{provider: p, model: model}, true
}

// Name implements Engine, returning the provider name.
func (s *Session) Name() string { return s.provider.Name }

// Generate runs the provider CLI once and returns its assistant text, streaming
// output to req.OnChunk as it arrives. The large grounded prompt is delivered
// on stdin when the provider supports it (avoiding ARG_MAX limits) and as a
// positional argument otherwise.
func (s *Session) Generate(ctx context.Context, req Request) (Result, error) {
	args := append([]string{}, s.provider.Args...)
	if s.provider.ModelFlag != "" && s.model != "" && s.model != "default" {
		args = append(args, s.provider.ModelFlag, s.model)
	}

	cmd := execCommand(ctx, s.provider.Bin, args...)
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
	var streamErr error
	if s.provider.StreamJSON {
		out, streamErr = parseStreamJSON(stdout, req.OnChunk)
	} else {
		out = streamAll(stdout, req.OnChunk)
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
	return Result{Text: strings.TrimSpace(out)}, nil
}

// parseStreamJSON reads the Claude Code stream-json event stream, forwarding
// each text delta to onChunk as it arrives (for a live preview) and returning
// the complete assistant text. The authoritative final `result` is preferred
// over the accumulated deltas; an error result is surfaced as an error.
func parseStreamJSON(r io.Reader, onChunk func(string)) (string, error) {
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
					Type string `json:"type"`
					Text string `json:"text"`
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
			if ev.Event.Type == "content_block_delta" && ev.Event.Delta.Type == "text_delta" && ev.Event.Delta.Text != "" {
				acc.WriteString(ev.Event.Delta.Text)
				if onChunk != nil {
					onChunk(ev.Event.Delta.Text)
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
		return acc.String(), err
	}
	if isError {
		if result == "" {
			result = "provider reported an error"
		}
		return "", fmt.Errorf("%s", result)
	}
	if haveResult && strings.TrimSpace(result) != "" {
		return result, nil
	}
	return acc.String(), nil
}

// streamAll reads r to completion, forwarding each chunk to onChunk if set, and
// returns the accumulated text.
func streamAll(r io.Reader, onChunk func(string)) string {
	var out strings.Builder
	buf := make([]byte, 4096)
	reader := bufio.NewReader(r)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			out.WriteString(chunk)
			if onChunk != nil {
				onChunk(chunk)
			}
		}
		if err != nil {
			break
		}
	}
	return out.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
