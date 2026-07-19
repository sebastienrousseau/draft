// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sebastienrousseau/draft/internal/config"
)

// Ollama generates text through a local Ollama server's /api/generate endpoint.
type Ollama struct {
	host    string
	extract string
	write   string
	edit    string
	numCtx  int
	numPred int
	client  *http.Client
}

// NewOllama builds an Ollama engine from configuration.
func NewOllama(cfg config.Config) *Ollama {
	return &Ollama{
		host:    strings.TrimRight(cfg.OllamaHost, "/"),
		extract: cfg.ExtractModel,
		write:   cfg.OllamaModel,
		edit:    cfg.EditModel,
		numCtx:  cfg.ContextLength,
		numPred: cfg.PredictLength,
		client:  http.DefaultClient,
	}
}

// Name implements Engine.
func (o *Ollama) Name() string { return "ollama" }

// Generate streams a single completion and reports whether the model stopped on
// a length limit (done_reason == "length"), which the pipeline uses to continue
// generation rather than fail on a mid-sentence cut.
func (o *Ollama) Generate(ctx context.Context, req Request) (Result, error) {
	payload := map[string]any{
		"model":  o.modelFor(req.Kind),
		"prompt": req.Prompt,
		"stream": true,
		"think":  false,
		"options": map[string]any{
			"temperature": req.Temperature,
			"num_ctx":     o.numCtx,
			"num_predict": o.numPred,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.host+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("ollama unreachable at %s: %w", o.host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return Result{}, fmt.Errorf("ollama http %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}

	var out strings.Builder
	var truncated bool
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var item struct {
			Response   string `json:"response"`
			Done       bool   `json:"done"`
			DoneReason string `json:"done_reason"`
			Error      string `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return Result{Text: out.String()}, err
		}
		if item.Error != "" {
			return Result{Text: out.String()}, fmt.Errorf("%s", item.Error)
		}
		if item.Response != "" {
			out.WriteString(item.Response)
			if req.OnChunk != nil {
				req.OnChunk(item.Response)
			}
		}
		if item.Done {
			truncated = item.DoneReason == "length"
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return Result{Text: out.String()}, err
	}
	return Result{Text: out.String(), Truncated: truncated}, nil
}

func (o *Ollama) modelFor(kind Kind) string {
	switch kind {
	case KindExtract:
		return o.extract
	case KindEdit:
		return o.edit
	default:
		return o.write
	}
}
