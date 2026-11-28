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
	"github.com/sebastienrousseau/draft/internal/gbnf"
	"sync"
)

// Bounds on how much of a response body is read for purposes other than the
// generated text itself, so a misbehaving server cannot make either unbounded.
const (
	maxErrorBodyBytes = 64 << 10
	maxDrainBytes     = 1 << 20
)

// Ollama generates text through a local Ollama server's /api/generate endpoint.
type Ollama struct {
	host       string
	ensureOnce sync.Once
	// schemaOnce guards the one-time probe of whether this server can
	// constrain the extraction model's output; schemaOK holds the answer.
	schemaOnce sync.Once
	schemaOK   bool
	// slots bounds the generations in flight to OllamaParallelism; see
	// acquireSlot.
	slots   chan struct{}
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
//
// Ollama sends response headers only once the model is loaded and the prompt
// evaluated, so the header wait has to cover a cold model load as well as a
// silent server.
func newOllamaClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			ResponseHeaderTimeout: ollamaHeaderTimeout,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   2,
		},
	}
}

// ollamaHeaderTimeout is how long Generate waits for the server to start
// answering. It was 60 seconds, and a cold 4B model (qwen3.5:4b) on an 8 GB
// machine took longer than that to load: the first extraction of a run
// failed with "timeout awaiting response headers" while the server was
// still loading, and only a warm model got through.
const ollamaHeaderTimeout = 5 * time.Minute

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
		slots:   make(chan struct{}, OllamaParallelism()),
	}
}

// ensureRunning starts the local server if needed. It is a variable so a test
// can substitute it without launching a process.
var ensureRunning = EnsureOllamaRunning

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

	// Start the local server on first use if it is not already up. The TUI did
	// this before a run; the headless and fallback paths did not, so a queue
	// that fell back to Ollama offline failed with "connection refused" even
	// though `ollama serve` would have answered. Done once per engine, and a
	// start failure is not fatal here: the request below reports the real
	// unreachable error with the host in it.
	o.ensureOnce.Do(func() { _ = ensureRunning(o.host) })

	body, err := o.requestBody(req)
	if err != nil {
		return Result{}, err
	}
	release, err := o.acquireSlot(ctx)
	if err != nil {
		return Result{}, err
	}
	defer release()
	resp, err := o.post(ctx, body)
	if err != nil {
		return Result{}, err
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
	return readOllamaStream(resp.Body, req.OnChunk)
}

// requestBody is the /api/generate payload for req.
func (o *Ollama) requestBody(req Request) ([]byte, error) {
	numPred := o.numPred
	if req.NumPredict > 0 && req.NumPredict < numPred {
		numPred = req.NumPredict
	}
	options := map[string]any{
		"temperature": req.Temperature,
		"num_ctx":     o.numCtx,
		"num_predict": numPred,
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
		"options":    options,
	}
	if req.Schema != nil {
		grammar, err := gbnf.FromJSONSchema(req.Schema)
		if err != nil {
			return nil, err
		}
		payload["format"] = req.Schema
		options["grammar"] = grammar
	}
	return json.Marshal(payload)
}

// post sends a generate request and returns the server's response.
func (o *Ollama) post(ctx context.Context, body []byte) (*http.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.host+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama unreachable at %s: %w", o.host, err)
	}
	return resp, nil
}

// readOllamaStream reads the streamed generate response, forwarding each chunk
// to onChunk, and reports whether the model stopped on a length limit.
func readOllamaStream(r io.Reader, onChunk func(string)) (Result, error) {
	var out strings.Builder
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var item struct {
			Response     string `json:"response"`
			Done         bool   `json:"done"`
			DoneReason   string `json:"done_reason"`
			Error        string `json:"error"`
			PromptTokens int    `json:"prompt_eval_count"`
			EvalTokens   int    `json:"eval_count"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return Result{Text: out.String()}, err
		}
		if item.Error != "" {
			return Result{Text: out.String()}, fmt.Errorf("%s", item.Error)
		}
		if item.Response != "" {
			out.WriteString(item.Response)
			if onChunk != nil {
				onChunk(item.Response)
			}
		}
		if item.Done {
			// The local model reports token counts but never a price.
			usage := Usage{InputTokens: item.PromptTokens, OutputTokens: item.EvalTokens, Reported: item.PromptTokens > 0 || item.EvalTokens > 0}
			return Result{Text: out.String(), Truncated: item.DoneReason == "length", Usage: usage}, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return Result{Text: out.String()}, err
	}
	return Result{Text: out.String()}, nil
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
