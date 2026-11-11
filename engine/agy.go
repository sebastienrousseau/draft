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
//
// A step_update whose step_type is "tool" ends the read with ErrToolUse. agy
// loads web, browser and subagent tools that no setting removes, so the deny
// rules in agyHermetic cannot be the whole guarantee: as with ACP, an answer
// from a turn that ran a tool is discarded rather than trusted.
func parseAgyStreamJSON(r io.Reader, onChunk func(string)) (string, Usage, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var turn agyTurn
	for scanner.Scan() {
		var ev agyEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue // agy may print a non-JSON banner line
		}
		if err := turn.apply(ev, onChunk); err != nil {
			return "", turn.usage, err
		}
	}
	if err := scanner.Err(); err != nil {
		return turn.acc.String(), turn.usage, err
	}
	return turn.answer()
}

// agyEvent is the part of one agy stream-json event that draft reads.
type agyEvent struct {
	Event      string `json:"event"`
	StepUpdate struct {
		StepType string `json:"step_type"`
		ToolName string `json:"tool_name"`
	} `json:"step_update"`
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

// agyTurn accumulates one turn's events into its answer.
type agyTurn struct {
	acc        strings.Builder
	response   string
	errMsg     string
	haveResult bool
	isError    bool
	usage      Usage
}

func (t *agyTurn) apply(ev agyEvent, onChunk func(string)) error {
	switch ev.Event {
	case "step_update":
		if ev.StepUpdate.StepType == "tool" {
			return fmt.Errorf("%w (%s)", ErrToolUse, ev.StepUpdate.ToolName)
		}
	case "assistant", "text", "message_delta", "content_block_delta":
		chunk := ev.Content.Text
		if chunk == "" {
			chunk = ev.Delta.Text
		}
		if chunk != "" {
			t.acc.WriteString(chunk)
			if onChunk != nil {
				onChunk(chunk)
			}
		}
	case "result":
		t.haveResult = true
		t.response = ev.Result.Response
		t.isError = strings.EqualFold(ev.Result.Status, "ERROR")
		t.errMsg = ev.Result.Error
		t.setUsage(ev.Result.Usage.InputTokens, ev.Result.Usage.OutputTokens, ev.Result.Usage.TotalTokens)
	}
	return nil
}

func (t *agyTurn) setUsage(in, out, total int) {
	if in == 0 && out == 0 && total > 0 {
		in = total // agy reports only a total
	}
	if in > 0 || out > 0 {
		t.usage = Usage{InputTokens: in, OutputTokens: out, Reported: true}
	}
}

// answer is the turn's text: the result event's response when there is one,
// the accumulated chunks otherwise.
func (t *agyTurn) answer() (string, Usage, error) {
	if t.isError {
		msg := t.errMsg
		if strings.TrimSpace(msg) == "" {
			msg = "agy reported an error"
		}
		return "", t.usage, fmt.Errorf("%s", firstLine(msg))
	}
	if t.haveResult && strings.TrimSpace(t.response) != "" {
		return strings.TrimSpace(t.response), t.usage, nil
	}
	return strings.TrimSpace(t.acc.String()), t.usage, nil
}
