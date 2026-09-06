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
func parseAgyStreamJSON(r io.Reader, onChunk func(string)) (string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var response, acc string
	var haveResult, isError bool
	var errMsg string
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
		}
	}
	if err := scanner.Err(); err != nil {
		return acc, err
	}
	if isError {
		if strings.TrimSpace(errMsg) == "" {
			errMsg = "agy reported an error"
		}
		return "", fmt.Errorf("%s", firstLine(errMsg))
	}
	if haveResult && strings.TrimSpace(response) != "" {
		return strings.TrimSpace(response), nil
	}
	return strings.TrimSpace(acc), nil
}
