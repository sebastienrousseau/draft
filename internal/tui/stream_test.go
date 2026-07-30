// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package tui

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// streamChunks approximates one 3000-word article arriving token by token.
func streamChunks() []string {
	out := make([]string, 0, 4400)
	for i := 0; i < 4000; i++ {
		out = append(out, "word ")
		if i%12 == 0 {
			out = append(out, "\n")
		}
	}
	return out
}

// appendToken runs once per streamed chunk — thousands of times per draft — so
// its cost must not grow with the length of the article already written. The
// previous implementation concatenated into a string and re-split the whole
// article for the preview on every chunk.
func BenchmarkTokenAppend(b *testing.B) {
	chunks := streamChunks()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var m Model
		for _, c := range chunks {
			m.appendToken(c)
		}
	}
}

func TestAppendTokenAccumulatesEverything(t *testing.T) {
	var m Model
	for _, c := range []string{"alpha ", "beta ", "gamma"} {
		m.appendToken(c)
	}
	if got := m.article(); got != "alpha beta gamma" {
		t.Errorf("article() = %q", got)
	}
	if m.words != 3 {
		t.Errorf("words = %d, want 3", m.words)
	}
}

// The preview window is bounded, but the full article must not be.
func TestAppendTokenBoundsTheTailNotTheArticle(t *testing.T) {
	var m Model
	chunk := strings.Repeat("x", 512) + "\n"
	total := 0
	for i := 0; i < 100; i++ { // ~51 KB, well past previewTailBytes
		m.appendToken(chunk)
		total += len(chunk)
	}

	if len(m.article()) != total {
		t.Errorf("article length = %d, want %d: accumulation must be unbounded", len(m.article()), total)
	}
	// The tail is bounded by lines, not bytes: previewTailBytes only applies
	// to output that arrives without line breaks to trim on.
	if got := strings.Count(string(m.tail), "\n"); got > previewLines {
		t.Errorf("tail holds %d lines, past the %d-line window", got, previewLines)
	}
	if strings.Count(m.preview, "\n")+1 > previewLines {
		t.Errorf("preview has more than %d lines", previewLines)
	}
}

// Output with no line breaks has nothing to trim on, so the byte backstop is
// what keeps the tail — and therefore the per-chunk cost — bounded.
func TestAppendTokenByteBackstopForLinelessOutput(t *testing.T) {
	var m Model
	for i := 0; i < 200; i++ {
		m.appendToken(strings.Repeat("y", 512)) // no newline anywhere
	}
	if len(m.tail) > previewTailBytes {
		t.Errorf("tail grew to %d bytes with no lines to trim on, past the %d cap", len(m.tail), previewTailBytes)
	}
	if len(m.article()) != 200*512 {
		t.Errorf("article length = %d; accumulation must still be unbounded", len(m.article()))
	}
}

// Trimming the tail must land on a line boundary, so the preview never opens
// mid-word — and never mid-rune.
func TestAppendTokenTrimsOnLineBoundaries(t *testing.T) {
	var m Model
	for i := 0; i < 400; i++ {
		m.appendToken("mesure très précise — 測定値です\n")
	}
	if !utf8.ValidString(string(m.tail)) {
		t.Error("tail is not valid UTF-8")
	}
	if !utf8.ValidString(m.preview) {
		t.Error("preview is not valid UTF-8")
	}
	for _, line := range strings.Split(strings.TrimSpace(m.preview), "\n") {
		if line != "mesure très précise — 測定値です" {
			t.Errorf("preview line was cut mid-line: %q", line)
			break
		}
	}
}

func TestResetOutputClearsBetweenJobs(t *testing.T) {
	var m Model
	m.appendToken("first job output\n")
	m.resetOutput()

	if m.article() != "" {
		t.Errorf("article() = %q after reset", m.article())
	}
	if m.preview != "" || m.words != 0 {
		t.Errorf("preview/words not cleared: %q / %d", m.preview, m.words)
	}

	m.appendToken("second job")
	if m.article() != "second job" {
		t.Errorf("article() = %q; the reset buffer must be reusable", m.article())
	}
}
