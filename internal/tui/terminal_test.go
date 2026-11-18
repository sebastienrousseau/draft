// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sebastienrousseau/draft/pipeline"
)

// payload is what a model, or a provider's error output, can put in front of
// the user: clear the screen, retitle the window, return to the line start.
const payload = "\x1b[2J\x1b]0;pwned\x07\rEVIL\u009b31m"

func hasControl(s string) bool {
	return strings.ContainsAny(s, "\x1b\x07\r\u009b")
}

// The streamed article, the log lines and a failed job's error all come from
// outside draft. None may carry a control sequence into the view; the
// article itself keeps every byte, because the preview is not the output.
func TestModelTextNeverReachesTheTerminalRaw(t *testing.T) {
	var m Model
	m.appendToken("streamed " + payload + "\n")
	if hasControl(m.preview) {
		t.Errorf("the preview carries a control sequence: %+q", m.preview)
	}
	if !strings.Contains(m.article(), payload) {
		t.Error("the streamed article lost bytes; only the preview is for the terminal")
	}

	m.appendLog("log " + payload)
	for _, l := range m.logs {
		if hasControl(l) {
			t.Errorf("a log line carries a control sequence: %+q", l)
		}
	}

	m2 := newModel(t, 1)
	m2 = upd(m2, tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 = upd(m2, pipeline.WarnEvent("warn "+payload))
	m2 = upd(m2, pipeline.ErrEvent("failed "+payload))
	// The view styles itself with SGR sequences, so ESC alone is not the test:
	// these are the payload's own controls.
	v := m2.View()
	for _, c := range []string{"\x07", "\x1b]", "\x1b[2J", "\r", "\u009b"} {
		if strings.Contains(v, c) {
			t.Errorf("the view carries %+q from the payload:\n%+q", c, v)
		}
	}
	if !strings.Contains(v, "failed [2J") {
		t.Errorf("the failure should still be shown, minus its controls:\n%s", v)
	}
}
