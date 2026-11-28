// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
)

// recordingEngine keeps every request it is sent and answers with out.
type recordingEngine struct {
	name   string
	schema bool
	out    string
	mu     sync.Mutex
	reqs   []engine.Request
	probed int
}

func (e *recordingEngine) Name() string { return e.name }
func (e *recordingEngine) SupportsSchema() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.probed++
	return e.schema
}

func (e *recordingEngine) probes() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.probed
}
func (e *recordingEngine) Generate(_ context.Context, req engine.Request) (engine.Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.reqs = append(e.reqs, req)
	return engine.Result{Text: e.out}, nil
}

func (e *recordingEngine) requests() []engine.Request {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]engine.Request(nil), e.reqs...)
}

// claims.Schema and internal/gbnf existed, but nothing in the pipeline set
// Request.Schema. With DRAFT_STRUCTURED_OUTPUT on, every extraction sent to a
// schema-capable engine carries the schema and the JSON prompt; any other
// engine keeps the text prompt. With it off, the default, nothing changes,
// and the engine is not even asked whether it can apply a schema, because for
// Ollama asking costs a request to the server.
func TestExtractionSendsTheSchemaOnlyWhenOptedInAndCapable(t *testing.T) {
	jsonOut := `[{"claim":"The system reached 12 pages per second.","source_quote":"The system reached 12 pages per second in testing.","type":"metric","strength":"demonstrated"}]`
	for _, tc := range []struct {
		name       string
		structured bool
		schema     bool
		out        string
		wantSchema bool
		wantProbe  bool
	}{
		{"opted in, capable", true, true, jsonOut, true, true},
		{"opted in, text-only", true, false, "NONE", false, true},
		{"default, capable", false, true, "NONE", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eng := &recordingEngine{name: "fake", schema: tc.schema, out: tc.out}
			cfg := config.Config{HomeDir: t.TempDir(), CacheDir: t.TempDir(), StructuredOutput: tc.structured}
			r := NewRunner(cfg, []engine.Engine{eng}, nil)
			records, _, err := r.extractClaims(context.Background(), Job{}, cacheTestSections(), t.TempDir(), r.chainFor(engine.KindExtract))
			if err != nil {
				t.Fatal(err)
			}
			reqs := eng.requests()
			if len(reqs) != len(cacheTestSections()) {
				t.Fatalf("got %d extraction requests, want one per section", len(reqs))
			}
			for _, req := range reqs {
				if got := req.Schema != nil; got != tc.wantSchema {
					t.Errorf("Schema set = %v, want %v", got, tc.wantSchema)
				}
				if asksJSON := strings.Contains(req.Prompt, `"source_quote"`); asksJSON != tc.wantSchema {
					t.Errorf("prompt asks for JSON = %v, want %v", asksJSON, tc.wantSchema)
				}
			}
			if probed := eng.probes() > 0; probed != tc.wantProbe {
				t.Errorf("engine asked about schema support = %v, want %v", probed, tc.wantProbe)
			}
			if tc.wantSchema && len(records) == 0 {
				t.Error("a verified JSON answer produced no claims")
			}
		})
	}
}

// The text and JSON prompts produce different output for the same section,
// so one must never be served from the other's cache entry.
func TestExtractionCacheKeyDependsOnThePromptVariant(t *testing.T) {
	cfg := config.Config{HomeDir: t.TempDir(), CacheDir: t.TempDir()}
	eng := &recordingEngine{name: "fake"}
	r := NewRunner(cfg, []engine.Engine{eng}, nil)
	body := cacheTestSections()[0].Body
	text := r.extractKey(body, eng, extractRequest(body, &recordingEngine{name: "fake"}, true))
	json := r.extractKey(body, eng, extractRequest(body, &recordingEngine{name: "fake", schema: true}, true))
	if text == "" || json == "" || text == json {
		t.Errorf("text and JSON extractions share a cache key: %q, %q", text, json)
	}
}
