// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sebastienrousseau/draft/config"
)

// apiEngineTo builds an api engine for name and points it at url (a test
// server) instead of the real provider endpoint.
func apiEngineTo(t *testing.T, name, url string, cfg config.Config) *API {
	t.Helper()
	a, ok := NewAPIEngine(name, cfg)
	if !ok {
		t.Fatalf("NewAPIEngine(%q) not ok", name)
	}
	a.endpoint = url
	return a
}

func TestAPIAnthropicHappyPath(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "secret-key")
	var gotAuth, gotVersion, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"content":[{"type":"text","text":"Grounded prose."}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":3}}`)
	}))
	defer srv.Close()

	a := apiEngineTo(t, "api:anthropic", srv.URL, config.Config{})
	res, err := a.Generate(context.Background(), Request{Kind: KindWrite, Prompt: "write it", Temperature: 0.6})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "Grounded prose." {
		t.Errorf("text = %q", res.Text)
	}
	if res.Truncated {
		t.Error("end_turn should not be truncated")
	}
	if res.Usage.InputTokens != 10 || res.Usage.OutputTokens != 3 || !res.Usage.Reported {
		t.Errorf("usage = %+v", res.Usage)
	}
	if gotAuth != "secret-key" || gotVersion != "2023-06-01" {
		t.Errorf("auth headers: key=%q version=%q", gotAuth, gotVersion)
	}
	if !strings.Contains(gotBody, `"claude-3-5-sonnet-latest"`) || !strings.Contains(gotBody, `"write it"`) {
		t.Errorf("request body wrong: %s", gotBody)
	}
}

func TestAPIAnthropicTruncated(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "k")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"content":[{"type":"text","text":"cut"}],"stop_reason":"max_tokens","usage":{}}`)
	}))
	defer srv.Close()
	a := apiEngineTo(t, "api:anthropic", srv.URL, config.Config{})
	res, err := a.Generate(context.Background(), Request{Prompt: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated {
		t.Error("max_tokens should report truncated")
	}
}

func TestAPIOpenAIHappyPath(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		io.WriteString(w, `{"choices":[{"message":{"content":"OpenAI text."},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`)
	}))
	defer srv.Close()

	a := apiEngineTo(t, "api:openai", srv.URL, config.Config{Model: "gpt-4o-mini"})
	res, err := a.Generate(context.Background(), Request{Prompt: "x", NumPredict: 100})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "OpenAI text." {
		t.Errorf("text = %q", res.Text)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("authorization = %q", gotAuth)
	}
	if res.Usage.InputTokens != 5 || res.Usage.OutputTokens != 2 {
		t.Errorf("usage = %+v", res.Usage)
	}
	if a.resolvedModel() != "gpt-4o-mini" {
		t.Errorf("model override not applied: %q", a.resolvedModel())
	}
}

func TestAPIOpenAILengthTruncatedAndNoChoices(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "k")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"c"},"finish_reason":"length"}]}`)
	}))
	defer srv.Close()
	a := apiEngineTo(t, "api:openai", srv.URL, config.Config{})
	res, err := a.Generate(context.Background(), Request{Prompt: "x"})
	if err != nil || !res.Truncated {
		t.Fatalf("res=%+v err=%v; want truncated", res, err)
	}

	// No choices is an error.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"choices":[]}`)
	}))
	defer srv2.Close()
	a2 := apiEngineTo(t, "api:openai", srv2.URL, config.Config{})
	if _, err := a2.Generate(context.Background(), Request{Prompt: "x"}); err == nil {
		t.Error("empty choices should error")
	}
}

func TestAPIMissingKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	a := apiEngineTo(t, "api:anthropic", "http://127.0.0.1:0", config.Config{})
	_, err := a.Generate(context.Background(), Request{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Errorf("missing key error = %v", err)
	}
}

func TestAPIHTTPErrorStatus(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "k")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()
	a := apiEngineTo(t, "api:anthropic", srv.URL, config.Config{})
	_, err := a.Generate(context.Background(), Request{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("http error = %v", err)
	}
}

func TestAPIProviderErrorField(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "k")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// 200 OK but an error payload.
		io.WriteString(w, `{"error":{"message":"overloaded"}}`)
	}))
	defer srv.Close()
	a := apiEngineTo(t, "api:anthropic", srv.URL, config.Config{})
	if _, err := a.Generate(context.Background(), Request{Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "overloaded") {
		t.Errorf("provider error = %v", err)
	}
}

func TestNewAPIEngineNaming(t *testing.T) {
	if _, ok := NewAPIEngine("claude", config.Config{}); ok {
		t.Error("a non-api name must not build an api engine")
	}
	if _, ok := NewAPIEngine("api:nope", config.Config{}); ok {
		t.Error("an unknown provider must not build")
	}
	a, ok := NewAPIEngine("api:anthropic", config.Config{})
	if !ok || a.Name() != "api:anthropic" {
		t.Errorf("build failed: ok=%v name=%q", ok, a.Name())
	}
	if a.resolvedModel() != "claude-3-5-sonnet-latest" {
		t.Errorf("default model = %q", a.resolvedModel())
	}
}

func TestValidateAcceptsAPINames(t *testing.T) {
	if err := Validate(config.Config{Engine: "api:anthropic"}); err != nil {
		t.Errorf("api:anthropic should validate: %v", err)
	}
	if err := Validate(config.Config{Engine: "api:openai"}); err != nil {
		t.Errorf("api:openai should validate: %v", err)
	}
	err := Validate(config.Config{Engine: "api:bogus"})
	if err == nil || !strings.Contains(err.Error(), "unknown api provider") {
		t.Errorf("api:bogus error = %v", err)
	}
}

func TestChainAndResolveForAPIEngine(t *testing.T) {
	cfg := config.Config{Engine: "api:anthropic", OllamaHost: config.OllamaHost}
	chain := ChainFor(cfg, KindWrite)
	if len(chain) != 2 || chain[0].Name() != "api:anthropic" || chain[1].Name() != "ollama" {
		t.Fatalf("chain = %v, want [api:anthropic ollama]", names(chain))
	}
	if got := ResolveModel(cfg, chain[0]); got != "claude-3-5-sonnet-latest" {
		t.Errorf("ResolveModel = %q", got)
	}
}

func TestAPIProviderNamesAndPrefixed(t *testing.T) {
	got := APIProviderNames()
	if len(got) != 2 || got[0] != "anthropic" || got[1] != "openai" {
		t.Errorf("provider names = %v, want sorted [anthropic openai]", got)
	}
	p := prefixed("api:", got)
	if p[0] != "api:anthropic" || p[1] != "api:openai" {
		t.Errorf("prefixed = %v", p)
	}
}

func TestTruncateBody(t *testing.T) {
	if truncateBody([]byte("short")) != "short" {
		t.Error("short body changed")
	}
	long := strings.Repeat("x", 3000)
	got := truncateBody([]byte(long))
	if !strings.HasSuffix(got, "…") || len(got) >= len(long) {
		t.Errorf("long body not truncated: len %d", len(got))
	}
}

// ensure the api engine satisfies Engine and its response marshals as expected.
var _ Engine = (*API)(nil)

func TestAPIRequestIsValidJSON(t *testing.T) {
	// A guard that the build funcs produce marshalable payloads.
	for _, p := range apiProviders {
		body := p.build("m", "prompt", 0.3, 100)
		if _, err := json.Marshal(body); err != nil {
			t.Errorf("%s payload not marshalable: %v", p.name, err)
		}
	}
}

func TestAPIRespectsTimeoutBranch(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "k")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{}}`)
	}))
	defer srv.Close()
	// A positive timeout exercises the context.WithTimeout branch.
	a := apiEngineTo(t, "api:anthropic", srv.URL, config.Config{CallTimeout: time.Minute})
	if _, err := a.Generate(context.Background(), Request{Prompt: "x"}); err != nil {
		t.Fatal(err)
	}
}

func TestAPIUnreachable(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "k")
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // now nothing listens
	a := apiEngineTo(t, "api:anthropic", url, config.Config{})
	if _, err := a.Generate(context.Background(), Request{Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("closed server error = %v", err)
	}
}

func TestAPIMalformedJSON(t *testing.T) {
	for _, name := range []string{"api:anthropic", "api:openai"} {
		env := "ANTHROPIC_API_KEY"
		if name == "api:openai" {
			env = "OPENAI_API_KEY"
		}
		t.Setenv(env, "k")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			io.WriteString(w, `{not valid json`)
		}))
		a := apiEngineTo(t, name, srv.URL, config.Config{})
		if _, err := a.Generate(context.Background(), Request{Prompt: "x"}); err == nil {
			t.Errorf("%s: malformed JSON should error", name)
		}
		srv.Close()
	}
}

func TestNewEngineDispatchesAPINames(t *testing.T) {
	if _, ok := NewEngine("api:anthropic", config.Config{}); !ok {
		t.Error("NewEngine should build a known api: engine")
	}
	if _, ok := NewEngine("api:nope", config.Config{}); ok {
		t.Error("NewEngine must not build an unknown api provider")
	}
}
