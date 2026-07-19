// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"context"
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
	return NewOllama(config.Config{OllamaHost: srv.URL, OllamaModel: "qwen3:4b", ExtractModel: "gemma3:4b", EditModel: "gemma3:4b"})
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
