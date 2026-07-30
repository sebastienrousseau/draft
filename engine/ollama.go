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
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sebastienrousseau/draft/config"
)

// Bounds on how much of a response body is read for purposes other than the
// generated text itself, so a misbehaving server cannot make either unbounded.
const (
	maxErrorBodyBytes = 64 << 10
	maxDrainBytes     = 1 << 20
)

// Ollama generates text through a local Ollama server's /api/generate endpoint.
type Ollama struct {
	host    string
	extract string
	write   string
	edit    string
	numCtx  int
	numPred int
	timeout time.Duration
	client  *http.Client
}

// newOllamaClient builds the HTTP client used for generation. It deliberately
// does not use http.DefaultClient: that client is process-global (so any
// setting we changed would leak into unrelated code) and has no timeout at all.
//
// Client.Timeout is also wrong here, because it bounds the whole exchange
// including the streamed body — and a legitimate article can stream for
// minutes. What must be bounded is the server going silent, so the limits sit
// on the dial and on the wait for response headers, and the overall call is
// bounded by the context instead.
func newOllamaClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			ResponseHeaderTimeout: 60 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   2,
		},
	}
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
		timeout: cfg.CallTimeout,
		client:  newOllamaClient(),
	}
}

// Name implements Engine.
func (o *Ollama) Name() string { return "ollama" }

// Generate streams a single completion and reports whether the model stopped on
// a length limit (done_reason == "length"), which the pipeline uses to continue
// generation rather than fail on a mid-sentence cut.
func (o *Ollama) Generate(ctx context.Context, req Request) (Result, error) {
	if o.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.timeout)
		defer cancel()
	}

	numPred := o.numPred
	if req.NumPredict > 0 && req.NumPredict < numPred {
		numPred = req.NumPredict
	}
	payload := map[string]any{
		"model":  o.modelFor(req.Kind),
		"prompt": req.Prompt,
		"stream": true,
		"think":  false,
		// keep_alive holds the model in memory between the per-section extraction
		// calls and the write, so a run is not slowed by repeated reloads even when
		// the server was started without an OLLAMA_KEEP_ALIVE default.
		"keep_alive": "10m",
		"options": map[string]any{
			"temperature": req.Temperature,
			"num_ctx":     o.numCtx,
			"num_predict": numPred,
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
	defer func() {
		// Drain what is left before closing so the connection can be reused;
		// the early break on done would otherwise abandon it mid-body. The cap
		// keeps a misbehaving server from making the drain itself unbounded.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
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
