// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sebastienrousseau/draft/internal/pipeline"
)

func TestRenderAllPhaseStates(t *testing.T) {
	m := newModel(t, 3)
	m = upd(m, tea.WindowSizeMsg{Width: 120, Height: 44})
	// Mark queue entries in each state.
	m.results[0].state = stateDone
	m.results[1].state = stateRunning
	m.results[2].state = stateFailed
	// Drive every phase through running so statusText and phaseLine cover all arms.
	for i := 0; i < pipeline.NumPhases; i++ {
		m = upd(m, pipeline.PhaseEvent{Index: i, Status: "running"})
		_ = m.View()
		m = upd(m, pipeline.PhaseEvent{Index: i, Status: "done"})
	}
	m.phases[pipeline.PhaseWrite] = "failed"
	v := m.View()
	if !strings.Contains(v, "DRAFT") {
		t.Error("view should render")
	}
}

func TestScrollViewShortAndTall(t *testing.T) {
	m := newModel(t, 1)
	// Tall terminal: everything fits, no scroll footer.
	m = upd(m, tea.WindowSizeMsg{Width: 120, Height: 200})
	if strings.Contains(m.View(), "scroll ") {
		t.Error("tall view should not show a scroll footer")
	}
	// Short terminal: content overflows, scroll footer appears.
	m = upd(m, tea.WindowSizeMsg{Width: 120, Height: 6})
	m = upd(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if !strings.Contains(m.View(), "scroll ") {
		t.Error("short view should show a scroll footer")
	}
}

func TestFailedSummaryRendersError(t *testing.T) {
	m := newModel(t, 1)
	m = upd(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = upd(m, pipeline.ErrEvent("something broke\nsecond line"))
	v := m.View()
	if !strings.Contains(v, "Queue complete") || !strings.Contains(v, "something broke") {
		t.Errorf("failed summary missing error: %s", v)
	}
}

func TestWaitForEventAndTick(t *testing.T) {
	ch := make(chan pipeline.Event, 1)
	ch <- pipeline.LogEvent("x")
	if msg := waitForEvent(ch)(); msg == nil {
		t.Error("waitForEvent should deliver a message")
	}
	if progressTick()() == nil {
		t.Error("progressTick should produce a frame message")
	}
}

func TestEnterWithEmptyInputIsNoop(t *testing.T) {
	m := newModel(t, 1)
	m = upd(m, pipeline.DoneEvent{OutputPath: "/o/a.md", Words: 500, Engine: "claude"})
	before := len(m.jobs)
	m = upd(m, tea.KeyMsg{Type: tea.KeyEnter}) // empty input
	if len(m.jobs) != before {
		t.Error("empty enter should not queue a job")
	}
}
