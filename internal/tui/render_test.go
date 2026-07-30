// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/brand"
	"github.com/sebastienrousseau/draft/pipeline"
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
	if !strings.Contains(v, brand.Wordmark) {
		t.Error("view should render")
	}
}

func TestLogoVisibleAtStandardTerminal(t *testing.T) {
	// 80x24 is the classic default terminal; the logo must show there and
	// the view must still fit without needing to scroll.
	m := newModel(t, 1)
	m = upd(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	v := m.View()
	if !strings.Contains(v, logoLines[0]) {
		t.Errorf("logo missing at 80x24:\n%s", v)
	}
	if strings.Contains(v, "scroll ") {
		t.Errorf("80x24 view should fit without scrolling:\n%s", v)
	}
	if got := len(strings.Split(v, "\n")); got > 24 {
		t.Errorf("view is %d lines, exceeds the 24-row terminal", got)
	}
	if !strings.Contains(v, "Validate and save") {
		t.Error("the whole pipeline must stay visible at 80x24")
	}
}

func TestViewFitsEveryHeight(t *testing.T) {
	// The running view must never need to scroll: at every terminal height
	// the layout gives up decoration, then sections, to fit what is left.
	for h := 20; h <= 60; h++ {
		m := newModel(t, 2)
		m = upd(m, tea.WindowSizeMsg{Width: 100, Height: h})
		m.logs = []string{"resolved 1 source file(s)", "read 8 section(s)", "claim section 3/8"}
		if v := m.View(); strings.Contains(v, "scroll ") {
			t.Errorf("height %d needs scrolling:\n%s", h, v)
		}
	}
}

func TestLogoDisabledByEnv(t *testing.T) {
	t.Setenv("DRAFT_SHOW_LOGO", "0")
	m := newModel(t, 1)
	m = upd(m, tea.WindowSizeMsg{Width: 120, Height: 50})
	if v := m.View(); strings.Contains(v, logoLines[0]) {
		t.Error("DRAFT_SHOW_LOGO=0 should suppress the logo")
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
	if !strings.Contains(v, "failures") || !strings.Contains(v, "something broke") {
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

func TestRenderEngineSelectViewOnlineAndOffline(t *testing.T) {
	m := newModel(t, 1)
	m.selectingEngine = true
	m = upd(m, tea.WindowSizeMsg{Width: 100, Height: 30})

	vOnline := m.View()
	if !strings.Contains(vOnline, "Select LLM Provider") || !strings.Contains(vOnline, "[Online]") {
		t.Errorf("online view missing expected content: %s", vOnline)
	}

	orig := engine.IsOnline
	engine.IsOnline = func() bool { return false }
	defer func() { engine.IsOnline = orig }()

	vOffline := m.View()
	if !strings.Contains(vOffline, "[Offline - Zero Network]") {
		t.Errorf("offline view missing offline header: %s", vOffline)
	}
}
