// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/internal/config"
)

func ollamaFor(t *testing.T, handler http.HandlerFunc) *Ollama {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewOllama(config.Config{OllamaHost: srv.URL, OllamaModel: "gemma3:4b", ExtractModel: "gemma3:4b", EditModel: "gemma3:4b"})
}

func TestOllamaGenerateSuccess(t *testing.T) {
	o := ollamaFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"response":"# Hi","done":false}` + "\n"))
		w.Write([]byte(`{"response":" there","done":false}` + "\n"))
		w.Write([]byte(`{"response":"","done":true,"done_reason":"stop"}` + "\n"))
	})
	var streamed strings.Builder
	res, err := o.Generate(context.Background(), Request{Kind: KindWrite, OnChunk: func(c string) { streamed.WriteString(c) }})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "# Hi there" {
		t.Errorf("text = %q", res.Text)
	}
	if res.Truncated {
		t.Error("should not be truncated on done_reason stop")
	}
	if streamed.String() != "# Hi there" {
		t.Errorf("streamed = %q", streamed.String())
	}
}

func TestOllamaHonoursNumPredictAndKeepAlive(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"response":"ok","done":true,"done_reason":"stop"}` + "\n"))
	}))
	t.Cleanup(srv.Close)
	o := NewOllama(config.Config{OllamaHost: srv.URL, OllamaModel: "gemma3:4b", ExtractModel: "gemma3:4b", EditModel: "gemma3:4b", PredictLength: 6000})

	// A per-request cap below the engine default must win; keep_alive must be set.
	if _, err := o.Generate(context.Background(), Request{Kind: KindWrite, NumPredict: 1500}); err != nil {
		t.Fatal(err)
	}
	if gotBody["keep_alive"] == nil {
		t.Error("request should set keep_alive to hold the model resident")
	}
	opts, _ := gotBody["options"].(map[string]any)
	if got, _ := opts["num_predict"].(float64); got != 1500 {
		t.Errorf("num_predict = %v, want 1500 (per-request cap)", got)
	}

	// A cap at or above the default leaves the engine default in place.
	if _, err := o.Generate(context.Background(), Request{Kind: KindWrite, NumPredict: 9000}); err != nil {
		t.Fatal(err)
	}
	opts, _ = gotBody["options"].(map[string]any)
	if got, _ := opts["num_predict"].(float64); got != 6000 {
		t.Errorf("num_predict = %v, want 6000 (engine default retained)", got)
	}
}

func TestOllamaTruncated(t *testing.T) {
	o := ollamaFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"response":"partial","done":true,"done_reason":"length"}` + "\n"))
	})
	res, err := o.Generate(context.Background(), Request{Kind: KindWrite})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated {
		t.Error("done_reason length should set Truncated")
	}
}

func TestOllamaStreamError(t *testing.T) {
	o := ollamaFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":"model not found"}` + "\n"))
	})
	if _, err := o.Generate(context.Background(), Request{}); err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Errorf("expected stream error, got %v", err)
	}
}

func TestOllamaHTTPError(t *testing.T) {
	o := ollamaFor(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	})
	if _, err := o.Generate(context.Background(), Request{}); err == nil || !strings.Contains(err.Error(), "ollama http") {
		t.Errorf("expected http error, got %v", err)
	}
}

func TestOllamaUnreachable(t *testing.T) {
	o := NewOllama(config.Config{OllamaHost: "http://127.0.0.1:0"})
	if _, err := o.Generate(context.Background(), Request{}); err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("expected unreachable error, got %v", err)
	}
}

func TestOllamaModelForKind(t *testing.T) {
	o := NewOllama(config.Config{OllamaModel: "w", ExtractModel: "e", EditModel: "d"})
	cases := map[Kind]string{KindWrite: "w", KindExtract: "e", KindEdit: "d"}
	for k, want := range cases {
		if got := o.modelFor(k); got != want {
			t.Errorf("modelFor(%d) = %q, want %q", k, got, want)
		}
	}
}

func TestOllamaBadJSON(t *testing.T) {
	o := ollamaFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("this is not json\n"))
	})
	if _, err := o.Generate(context.Background(), Request{}); err == nil {
		t.Error("malformed stream line should error")
	}
}
