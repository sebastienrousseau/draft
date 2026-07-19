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

// Claude generates text through the `claude` CLI in headless print mode. It
// relies on the user's existing session for authentication, so no API token is
// ever read or required.
type Claude struct {
	model string
}

// NewClaude builds a Claude engine from configuration.
func NewClaude(cfg config.Config) *Claude {
	return &Claude{model: strings.TrimSpace(cfg.ClaudeModel)}
}

// Name implements Engine.
func (c *Claude) Name() string { return "claude" }

// Generate streams the prompt to `claude -p` and returns the assistant text.
// The prompt is written to stdin (not passed as an argument) to avoid ARG_MAX
// limits on the large grounded prompts this tool builds.
func (c *Claude) Generate(ctx context.Context, req Request) (Result, error) {
	args := []string{"-p", "--output-format", "text"}
	if c.model != "" && c.model != "default" {
		args = append(args, "--model", c.model)
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = strings.NewReader(req.Prompt)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return Result{}, err
	}

	var out strings.Builder
	buf := make([]byte, 4096)
	reader := bufio.NewReader(stdout)
	for {
		n, rerr := reader.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			out.WriteString(chunk)
			if req.OnChunk != nil {
				req.OnChunk(chunk)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			// Drain and surface as an error after Wait reports exit status.
			break
		}
	}

	if err := cmd.Wait(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return Result{}, fmt.Errorf("claude: %s", msg)
	}
	return Result{Text: strings.TrimSpace(out.String())}, nil
}
