// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/sebastienrousseau/draft/config"
)

// APIPrefix marks an engine name as the direct-API escape hatch:
// "api:anthropic", "api:openai". It is opt-in and never chosen by auto mode —
// draft's default drives an agent CLI's logged-in session and reads no key.
// This exists only for users who have no agent CLI installed and would rather
// supply an API key of their own.
const APIPrefix = "api:"

// apiProvider describes one HTTP chat API: where to send a prompt, how to
// authenticate, and how to read the answer. The two shapes draft speaks —
// Anthropic Messages and OpenAI Chat Completions — differ enough in request and
// response that each carries its own build and parse.
type apiProvider struct {
	name         string // short name after the prefix, e.g. "anthropic"
	endpoint     string
	keyEnv       string // environment variable holding the API key
	defaultModel string
	// authorize sets the provider's auth header(s) on the request.
	authorize func(req *http.Request, key string)
	// build returns the JSON request body for a prompt.
	build func(model, prompt string, temperature float64, maxTokens int) map[string]any
	// parse reads the response body into text, whether the output was cut at
	// the token limit, and the usage the provider reported.
	parse func(body []byte) (text string, truncated bool, usage Usage, err error)
}

// apiProviders is the registry of supported direct APIs, keyed by short name.
var apiProviders = map[string]apiProvider{
	"anthropic": {
		name:         "anthropic",
		endpoint:     "https://api.anthropic.com/v1/messages",
		keyEnv:       "ANTHROPIC_API_KEY",
		defaultModel: "claude-3-5-sonnet-latest",
		authorize: func(req *http.Request, key string) {
			req.Header.Set("x-api-key", key)
			req.Header.Set("anthropic-version", "2023-06-01")
		},
		build: func(model, prompt string, temperature float64, maxTokens int) map[string]any {
			return map[string]any{
				"model":       model,
				"max_tokens":  maxTokens,
				"temperature": temperature,
				"messages":    []map[string]any{{"role": "user", "content": prompt}},
			}
		},
		parse: parseAnthropic,
	},
	"openai": {
		name:         "openai",
		endpoint:     "https://api.openai.com/v1/chat/completions",
		keyEnv:       "OPENAI_API_KEY",
		defaultModel: "gpt-4o",
		authorize: func(req *http.Request, key string) {
			req.Header.Set("Authorization", "Bearer "+key)
		},
		build: func(model, prompt string, temperature float64, maxTokens int) map[string]any {
			return map[string]any{
				"model":       model,
				"temperature": temperature,
				"max_tokens":  maxTokens,
				"messages":    []map[string]any{{"role": "user", "content": prompt}},
			}
		},
		parse: parseOpenAI,
	},
}

// apiDefaultMaxTokens caps output when a request does not size it. An article
// is long, so the ceiling is generous; NumPredict lowers it per call.
const apiDefaultMaxTokens = 8192

// API generates text through a hosted chat API using the user's own API key.
type API struct {
	provider apiProvider
	model    string
	endpoint string // overridable for tests
	timeout  time.Duration
	client   *http.Client
}

// APIProviderNames lists the supported direct-API provider short names, sorted,
// for help and error messages.
func APIProviderNames() []string {
	names := make([]string, 0, len(apiProviders))
	for n := range apiProviders {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// isAPIName reports whether name selects the direct-API escape hatch, and
// returns the provider short name.
func isAPIName(name string) (provider string, ok bool) {
	if !strings.HasPrefix(name, APIPrefix) {
		return "", false
	}
	return strings.TrimPrefix(name, APIPrefix), true
}

// NewAPIEngine builds a direct-API engine from a name like "api:anthropic". It
// returns false for a name that is not an api: reference to a known provider.
// A missing API key is not an error here — the name is valid — but surfaces at
// call time, so an unavailable key degrades the chain to Ollama rather than
// failing configuration.
func NewAPIEngine(name string, cfg config.Config) (*API, bool) {
	short, ok := isAPIName(name)
	if !ok {
		return nil, false
	}
	p, ok := apiProviders[short]
	if !ok {
		return nil, false
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = p.defaultModel
	}
	return &API{
		provider: p,
		model:    model,
		endpoint: p.endpoint,
		timeout:  cfg.CallTimeout,
		client:   newAPIClient(),
	}, true
}

// newAPIClient bounds the dial and idle connection but not the wait for the
// full response: a hosted model can take minutes to write an article, and the
// overall call is bounded by the context instead (see Generate).
func newAPIClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			IdleConnTimeout:     90 * time.Second,
			MaxIdleConnsPerHost: 2,
		},
	}
}

// Name implements Engine: the full escape-hatch name, so routing and provenance
// record that a hosted API — not an agent session — produced the text.
func (a *API) Name() string { return APIPrefix + a.provider.name }

// resolvedModel is the model label this engine uses, for ResolveModel.
func (a *API) resolvedModel() string { return a.model }

// Generate implements Engine: one request to the hosted API, the answer, and
// the usage it reported.
func (a *API) Generate(ctx context.Context, req Request) (Result, error) {
	if a.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.timeout)
		defer cancel()
	}
	key := strings.TrimSpace(os.Getenv(a.provider.keyEnv))
	if key == "" {
		return Result{}, fmt.Errorf("%s: %s is not set; export your API key or use an agent CLI (the keyless default)", a.Name(), a.provider.keyEnv)
	}

	maxTokens := apiDefaultMaxTokens
	if req.NumPredict > 0 && req.NumPredict < maxTokens {
		maxTokens = req.NumPredict
	}
	payload, err := json.Marshal(a.provider.build(a.model, req.Prompt, req.Temperature, maxTokens))
	if err != nil {
		return Result{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(payload))
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	a.provider.authorize(httpReq, key)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("%s unreachable: %w", a.Name(), err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes))
		_ = resp.Body.Close()
	}()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDrainBytes))
	if err != nil {
		return Result{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("%s http %s: %s", a.Name(), resp.Status, strings.TrimSpace(truncateBody(data)))
	}
	text, truncated, usage, err := a.provider.parse(data)
	if err != nil {
		return Result{}, fmt.Errorf("%s: %w", a.Name(), err)
	}
	return Result{Text: strings.TrimSpace(text), Truncated: truncated, Usage: usage}, nil
}

// parseAnthropic reads an Anthropic Messages response.
func parseAnthropic(body []byte) (string, bool, Usage, error) {
	var r struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", false, Usage{}, err
	}
	if r.Error != nil {
		return "", false, Usage{}, fmt.Errorf("%s", r.Error.Message)
	}
	var b strings.Builder
	for _, c := range r.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	usage := Usage{InputTokens: r.Usage.InputTokens, OutputTokens: r.Usage.OutputTokens, Reported: r.Usage.InputTokens > 0 || r.Usage.OutputTokens > 0}
	return b.String(), r.StopReason == "max_tokens", usage, nil
}

// parseOpenAI reads an OpenAI Chat Completions response.
func parseOpenAI(body []byte) (string, bool, Usage, error) {
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", false, Usage{}, err
	}
	if r.Error != nil {
		return "", false, Usage{}, fmt.Errorf("%s", r.Error.Message)
	}
	if len(r.Choices) == 0 {
		return "", false, Usage{}, fmt.Errorf("no choices in response")
	}
	usage := Usage{InputTokens: r.Usage.PromptTokens, OutputTokens: r.Usage.CompletionTokens, Reported: r.Usage.PromptTokens > 0 || r.Usage.CompletionTokens > 0}
	return r.Choices[0].Message.Content, r.Choices[0].FinishReason == "length", usage, nil
}

// truncateBody caps an error body so a hostile server cannot make the message
// unbounded.
func truncateBody(b []byte) string {
	const max = 2000
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}

// prefixed returns each name with prefix prepended, for building the
// "api:anthropic, api:openai" style hint in error messages.
func prefixed(prefix string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}
