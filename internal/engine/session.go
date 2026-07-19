package engine

import (
	"bufio"
	"context"
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

	out := streamAll(stdout, req.OnChunk)

	if err := cmd.Wait(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return Result{}, fmt.Errorf("%s: %s", s.provider.Name, firstLine(msg))
	}
	return Result{Text: strings.TrimSpace(out)}, nil
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
