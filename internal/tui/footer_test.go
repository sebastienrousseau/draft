// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package tui

import (
	"strings"
	"testing"
)

// The footer names the backend actually in use, so a run that fell back to
// Ollama is visible without reading the log pane.
func TestEngineFootnote(t *testing.T) {
	var m Model
	if got := m.engineFootnote(); got != "draft" {
		t.Errorf("with no engine yet: got %q, want %q", got, "draft")
	}
	m.engineName = "ollama"
	if got := m.engineFootnote(); got != "draft · ollama" {
		t.Errorf("got %q", got)
	}
}

// The footer must stay on one line whatever the terminal width, including
// widths too narrow to fit both halves.
func TestRenderFooterFitsAnyWidth(t *testing.T) {
	var m Model
	m.engineName = "cursor-agent"
	for _, width := range []int{0, 1, 20, 80, 200} {
		got := m.renderFooter(width)
		if strings.Contains(got, "\n") {
			t.Errorf("width %d: footer wrapped onto a second line", width)
		}
		if !strings.Contains(got, "cursor-agent") {
			t.Errorf("width %d: engine name missing from %q", width, got)
		}
	}
}
