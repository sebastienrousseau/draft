// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// Session used to return Truncated=false unconditionally, so the pipeline's
// continuation machinery could never fire for a session provider and a
// length-limited stop surfaced much later as a rule violation. The stop reason
// is on the message_delta event.
func TestParseStreamJSONReportsLengthStop(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Half an "}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"article"}}}`,
		`{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"max_tokens"}}}`,
	}, "\n")

	text, truncated, _, err := parseStreamJSON(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatalf("parseStreamJSON: %v", err)
	}
	if text != "Half an article" {
		t.Errorf("text = %q", text)
	}
	if !truncated {
		t.Error("truncated = false; a max_tokens stop must be reported")
	}
}

func TestParseStreamJSONCleanStopIsNotTruncated(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Done."}}}`,
		`{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"}}}`,
		`{"type":"result","result":"Done."}`,
	}, "\n")

	text, truncated, _, err := parseStreamJSON(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatalf("parseStreamJSON: %v", err)
	}
	if truncated {
		t.Error("truncated = true for an end_turn stop")
	}
	if text != "Done." {
		t.Errorf("text = %q, want the authoritative result", text)
	}
}

// A truncated stream that also carries a final result must still report the
// truncation: the result is authoritative for the text, not for the stop.
func TestParseStreamJSONTruncationSurvivesResult(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"max_tokens"}}}`,
		`{"type":"result","result":"partial text"}`,
	}, "\n")

	text, truncated, _, err := parseStreamJSON(strings.NewReader(stream), nil)
	if err != nil || !truncated || text != "partial text" {
		t.Errorf("got (%q, %v, %v)", text, truncated, err)
	}
}

func TestParseStreamJSONForwardsChunks(t *testing.T) {
	stream := `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"tick"}}}`
	var got []string
	if _, _, _, err := parseStreamJSON(strings.NewReader(stream), func(s string) { got = append(got, s) }); err != nil {
		t.Fatalf("parseStreamJSON: %v", err)
	}
	if len(got) != 1 || got[0] != "tick" {
		t.Errorf("onChunk received %v", got)
	}
}

func TestParseStreamJSONDefaultErrorAndReadFailure(t *testing.T) {
	_, _, _, err := parseStreamJSON(strings.NewReader(`{"type":"result","is_error":true}`), nil)
	if err == nil || err.Error() != "provider reported an error" {
		t.Fatalf("empty provider error = %v", err)
	}
	text, _, _, err := parseStreamJSON(&errReader{data: `{"type":"stream_event"}`}, nil)
	if !errors.Is(err, errBrokenPipe) || text != "" {
		t.Fatalf("read failure = (%q, %v)", text, err)
	}
}

// A refusal arrives as a stop reason, on the message_delta event or on the
// final result, with no text and is_error false. It must come back as the
// ErrRefused sentinel so the pipeline can tell a declined prompt from a dead
// provider; a generic error here is what demoted whole queues.
func TestParseStreamJSONRefusalIsTheSentinel(t *testing.T) {
	cases := map[string]string{
		"on the result": strings.Join([]string{
			`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed"}}`,
			`{"type":"result","subtype":"success","is_error":false,"result":"","stop_reason":"refusal"}`,
		}, "\n"),
		"on message_delta": strings.Join([]string{
			`{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"refusal"}}}`,
			`{"type":"result","subtype":"success","is_error":false,"result":""}`,
		}, "\n"),
	}
	for name, stream := range cases {
		t.Run(name, func(t *testing.T) {
			text, _, _, err := parseStreamJSON(strings.NewReader(stream), nil)
			if !errors.Is(err, ErrRefused) {
				t.Fatalf("err = %v, want ErrRefused", err)
			}
			if text != "" {
				t.Errorf("text = %q, want none for a declined prompt", text)
			}
		})
	}
}

// errReader returns some data and then a non-EOF failure.
type errReader struct {
	data string
	done bool
}

var errBrokenPipe = errors.New("broken pipe")

func (e *errReader) Read(p []byte) (int, error) {
	if e.done {
		return 0, errBrokenPipe
	}
	e.done = true
	return copy(p, e.data), nil
}

// streamAll used to swallow read errors, so a broken pipe produced a
// half-written article that looked complete to everything downstream.
func TestStreamAllReportsReadError(t *testing.T) {
	text, err := streamAll(&errReader{data: "partial"}, nil)
	if !errors.Is(err, errBrokenPipe) {
		t.Errorf("err = %v, want the underlying read error", err)
	}
	if text != "partial" {
		t.Errorf("text = %q; what was read should still be returned", text)
	}
}

func TestStreamAllEOFIsNotAnError(t *testing.T) {
	text, err := streamAll(strings.NewReader("all of it"), nil)
	if err != nil {
		t.Errorf("err = %v, want nil for a clean EOF", err)
	}
	if text != "all of it" {
		t.Errorf("text = %q", text)
	}
}

func TestStreamAllForwardsChunks(t *testing.T) {
	var got strings.Builder
	if _, err := streamAll(io.NopCloser(strings.NewReader("abc")), func(s string) { got.WriteString(s) }); err != nil {
		t.Fatalf("streamAll: %v", err)
	}
	if got.String() != "abc" {
		t.Errorf("onChunk received %q", got.String())
	}
}

// The result event carries the run's cost and token counts; parseStreamJSON
// surfaces them so the pipeline can total what a job spent.
func TestParseStreamJSONReportsUsage(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"hi","total_cost_usd":0.0123,"usage":{"input_tokens":1200,"output_tokens":340}}`,
	}, "\n")
	_, _, usage, err := parseStreamJSON(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !usage.Reported || usage.InputTokens != 1200 || usage.OutputTokens != 340 || usage.CostUSD != 0.0123 {
		t.Errorf("usage = %+v", usage)
	}
	// A result with no usage leaves it unreported.
	_, _, usage, _ = parseStreamJSON(strings.NewReader(`{"type":"result","result":"x"}`), nil)
	if usage.Reported {
		t.Errorf("usage should be unreported, got %+v", usage)
	}
}

func TestUsageAdd(t *testing.T) {
	var u Usage
	u.Add(Usage{InputTokens: 10, OutputTokens: 5, CostUSD: 0.1, Reported: true})
	u.Add(Usage{InputTokens: 3, OutputTokens: 2, CostUSD: 0.02, Reported: true})
	if u.InputTokens != 13 || u.OutputTokens != 7 || u.CostUSD < 0.1199 || u.CostUSD > 0.1201 || !u.Reported {
		t.Errorf("accumulated = %+v", u)
	}
	// Adding a silent call keeps Reported true and adds zero.
	u.Add(Usage{})
	if u.InputTokens != 13 || !u.Reported {
		t.Errorf("silent add changed the total: %+v", u)
	}
}
