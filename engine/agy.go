// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// agyUserEvent wraps a prompt as the single NDJSON "user" event agy's
// stream-json input mode expects on stdin. Delivering it this way keeps the
// verbatim source text out of the process listing, which a positional prompt
// argument would expose. The shape is fixed and holds only strings, so
// marshalling cannot fail; the text is JSON-escaped by the encoder.
func agyUserEvent(prompt string) string {
	evt := map[string]any{
		"event": "user",
		"message": map[string]any{
			"role":    "user",
			"content": []map[string]any{{"type": "text", "text": prompt}},
		},
	}
	b, _ := json.Marshal(evt)
	return string(b) + "\n"
}

// parseAgyStreamJSON reads agy's NDJSON output and returns the final turn's
// text. agy emits a stream of events and closes with a "result" event whose
// result.status is SUCCESS or ERROR and whose result.response is the answer.
// Assistant text chunks, when present, are forwarded to onChunk for a live
// preview; the authoritative answer is the result event's response.
func parseAgyStreamJSON(r io.Reader, onChunk func(string)) (string, Usage, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var response, acc string
	var haveResult, isError bool
	var errMsg string
	var usage Usage
	for scanner.Scan() {
		line := scanner.Bytes()
		var ev struct {
			Event   string `json:"event"`
			Content struct {
				Text string `json:"text"`
			} `json:"content"`
			Delta struct {
				Text string `json:"text"`
			} `json:"delta"`
			Result struct {
				Status   string `json:"status"`
				Response string `json:"response"`
				Error    string `json:"error"`
				Usage    struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
					TotalTokens  int `json:"total_tokens"`
				} `json:"usage"`
			} `json:"result"`
		}
		if err := json.Unmarshal(line, &ev); err != nil {
			continue // agy may print a non-JSON banner line
		}
		switch ev.Event {
		case "assistant", "text", "message_delta", "content_block_delta":
			chunk := ev.Content.Text
			if chunk == "" {
				chunk = ev.Delta.Text
			}
			if chunk != "" {
				acc += chunk
				if onChunk != nil {
					onChunk(chunk)
				}
			}
		case "result":
			haveResult = true
			response = ev.Result.Response
			isError = strings.EqualFold(ev.Result.Status, "ERROR")
			errMsg = ev.Result.Error
			in, out := ev.Result.Usage.InputTokens, ev.Result.Usage.OutputTokens
			if in == 0 && out == 0 && ev.Result.Usage.TotalTokens > 0 {
				in = ev.Result.Usage.TotalTokens // agy reports only a total
			}
			if in > 0 || out > 0 {
				usage = Usage{InputTokens: in, OutputTokens: out, Reported: true}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return acc, usage, err
	}
	if isError {
		if strings.TrimSpace(errMsg) == "" {
			errMsg = "agy reported an error"
		}
		return "", usage, fmt.Errorf("%s", firstLine(errMsg))
	}
	if haveResult && strings.TrimSpace(response) != "" {
		return strings.TrimSpace(response), usage, nil
	}
	return strings.TrimSpace(acc), usage, nil
}
