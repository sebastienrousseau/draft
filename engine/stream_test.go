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

	text, truncated, err := parseStreamJSON(strings.NewReader(stream), nil)
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

	text, truncated, err := parseStreamJSON(strings.NewReader(stream), nil)
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

	text, truncated, err := parseStreamJSON(strings.NewReader(stream), nil)
	if err != nil || !truncated || text != "partial text" {
		t.Errorf("got (%q, %v, %v)", text, truncated, err)
	}
}

func TestParseStreamJSONForwardsChunks(t *testing.T) {
	stream := `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"tick"}}}`
	var got []string
	if _, _, err := parseStreamJSON(strings.NewReader(stream), func(s string) { got = append(got, s) }); err != nil {
		t.Fatalf("parseStreamJSON: %v", err)
	}
	if len(got) != 1 || got[0] != "tick" {
		t.Errorf("onChunk received %v", got)
	}
}

func TestParseStreamJSONDefaultErrorAndReadFailure(t *testing.T) {
	_, _, err := parseStreamJSON(strings.NewReader(`{"type":"result","is_error":true}`), nil)
	if err == nil || err.Error() != "provider reported an error" {
		t.Fatalf("empty provider error = %v", err)
	}
	text, _, err := parseStreamJSON(&errReader{data: `{"type":"stream_event"}`}, nil)
	if !errors.Is(err, errBrokenPipe) || text != "" {
		t.Fatalf("read failure = (%q, %v)", text, err)
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
