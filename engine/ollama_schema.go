// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// schemaProbeTimeout bounds the capability probe. A server that can apply a
// schema loads the extraction model to answer, which a cold start on a small
// machine can take a minute or more over; a server that cannot refuses at
// once. Timing out only means falling back to the text prompt.
const schemaProbeTimeout = 3 * time.Minute

// SupportsSchema implements SchemaCapable. Whether Ollama can hold output to
// a JSON Schema depends on the model and the runtime serving it, not on the
// server version: some runtimes answer any format with 501 "structured
// output is unavailable". So the first call asks the server once, with a
// one-token request carrying a format, and every later call reuses the
// answer. Any failure reads as "no", which keeps the text prompt that worked
// before structured output was wired in.
func (o *Ollama) SupportsSchema() bool {
	o.schemaOnce.Do(func() { o.schemaOK = o.probeSchema() })
	return o.schemaOK
}

func (o *Ollama) probeSchema() bool {
	o.ensureOnce.Do(func() { _ = ensureRunning(o.host) })
	body, err := json.Marshal(map[string]any{
		"model":      o.modelFor(KindExtract),
		"prompt":     "Return an empty JSON array.",
		"stream":     false,
		"think":      false,
		"format":     map[string]any{"type": "array"},
		"keep_alive": "10m",
		"options":    map[string]any{"num_predict": 1},
	})
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), schemaProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.host+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return false
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes))
		_ = resp.Body.Close()
	}()
	return resp.StatusCode == http.StatusOK
}
