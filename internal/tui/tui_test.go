package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sebastienrousseau/draft/internal/config"
	"github.com/sebastienrousseau/draft/internal/engine"
	"github.com/sebastienrousseau/draft/internal/pipeline"
)

type fakeEngine struct{ name string }

func (f fakeEngine) Name() string { return f.name }
func (f fakeEngine) Generate(context.Context, engine.Request) (engine.Result, error) {
	return engine.Result{}, nil
}

func newModel(t *testing.T, jobs int) Model {
	t.Helper()
	js := make([]pipeline.Job, jobs)
	for i := range js {
		js[i] = pipeline.Job{Sources: []string{"/tmp/x.pdf"}}
	}
	cfg := config.Config{OllamaModel: "qwen3:4b", HomeDir: "/home/seb"}
	return New(context.Background(), cfg, []engine.Engine{fakeEngine{"claude"}}, js)
}

func upd(m Model, msg tea.Msg) Model {
	tm, _ := m.Update(msg)
	return tm.(Model)
}

func TestInitAndSize(t *testing.T) {
	m := newModel(t, 2)
	if cmd := m.Init(); cmd == nil {
		t.Error("Init should return a command batch")
	}
	m = upd(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.width != 120 || m.height != 40 {
		t.Errorf("size not stored: %d x %d", m.width, m.height)
	}
	if m.results[0].state != stateRunning {
		t.Error("first job should be marked running")
	}
}

func TestPipelineEventsAndView(t *testing.T) {
	m := newModel(t, 1)
	m = upd(m, tea.WindowSizeMsg{Width: 120, Height: 44})
	m = upd(m, pipeline.PhaseEvent{Index: pipeline.PhaseWrite, Status: "running"})
	if m.genStarted.IsZero() {
		t.Error("write phase should start the focus timer")
	}
	m = upd(m, pipeline.LogEvent("hello log"))
	m = upd(m, pipeline.EngineEvent("codex"))
	if m.engineName != "codex" {
		t.Errorf("engine event not applied: %q", m.engineName)
	}
	m = upd(m, pipeline.TokenEvent("# Draft\n\n<aside x>\n\nExecutive Summary\n\n## S\n\nbody words here"))
	if m.words == 0 {
		t.Error("tokens should update word count")
	}
	if v := m.View(); !strings.Contains(v, "DRAFT") || !strings.Contains(v, "Live Draft") {
		t.Error("running view should render header and preview")
	}
	// Percentage must appear on the progress bar.
	if !strings.Contains(m.View(), "%") {
		t.Error("progress percentage missing")
	}
}

func TestManyLogsTrimmed(t *testing.T) {
	m := newModel(t, 1)
	for i := 0; i < 20; i++ {
		m = upd(m, pipeline.LogEvent("line"))
	}
	if len(m.logs) > 8 {
		t.Errorf("logs should be capped, got %d", len(m.logs))
	}
}

func TestDoneAdvancesQueueThenAllDone(t *testing.T) {
	m := newModel(t, 2)
	m = upd(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = upd(m, pipeline.DoneEvent{OutputPath: "/out/a.md", Words: 900, Engine: "claude"})
	if m.results[0].state != stateDone {
		t.Error("first job should be done")
	}
	if m.allDone {
		t.Error("should not be all-done with a job remaining")
	}
	m = upd(m, pipeline.ErrEvent("second failed"))
	if !m.allDone {
		t.Error("should be all-done after last job")
	}
	v := m.View()
	if !strings.Contains(v, "Queue complete") || !strings.Contains(v, "failed") {
		t.Errorf("summary view wrong:\n%s", v)
	}
}

func TestQueueAnotherSource(t *testing.T) {
	m := newModel(t, 1)
	m = upd(m, pipeline.DoneEvent{OutputPath: "/out/a.md", Words: 500, Engine: "claude"})
	if !m.allDone {
		t.Fatal("expected all-done")
	}
	// Type a path then press enter to queue a new job.
	m = upd(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/tmp/new.pdf")})
	m = upd(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.jobs) != 2 || m.allDone {
		t.Errorf("enter should queue a new job and resume, jobs=%d allDone=%v", len(m.jobs), m.allDone)
	}
}

func TestScrollKeys(t *testing.T) {
	m := newModel(t, 1)
	m = upd(m, tea.WindowSizeMsg{Width: 100, Height: 10})
	for _, k := range []string{"j", "down", "pgdown", " ", "end"} {
		m = upd(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
	if m.scroll == 0 {
		t.Error("scroll should have advanced")
	}
	m = upd(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("home")})
	// home maps via String(); use explicit keys for reliability:
	m = upd(m, tea.KeyMsg{Type: tea.KeyHome})
	if m.scroll != 0 {
		t.Errorf("home should reset scroll, got %d", m.scroll)
	}
	m = upd(m, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	m = upd(m, tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
}

func TestQuitKeys(t *testing.T) {
	m := newModel(t, 1)
	for _, key := range []tea.KeyMsg{{Type: tea.KeyCtrlC}, {Type: tea.KeyEsc}, {Type: tea.KeyRunes, Runes: []rune("q")}} {
		if _, cmd := m.Update(key); cmd == nil {
			t.Errorf("key %v should quit", key)
		}
	}
}

func TestSpinnerAndProgressTicks(t *testing.T) {
	m := newModel(t, 1)
	m = upd(m, spinner.TickMsg{})
	m = upd(m, progress.FrameMsg{})
	_ = m.View()
}

func TestViewDefaultsWhenUnsized(t *testing.T) {
	m := newModel(t, 1)
	if v := m.View(); v == "" {
		t.Error("view should render even before a size message")
	}
}

func TestHeaderFitsNarrow(t *testing.T) {
	m := newModel(t, 1)
	m = upd(m, tea.WindowSizeMsg{Width: 60, Height: 40})
	h := m.renderHeader(56)
	for _, line := range strings.Split(h, "\n") {
		// No rendered line may exceed the width (no bleed).
		if lipgloss.Width(line) > 56 {
			t.Errorf("header line bleeds: width %d > 56: %q", lipgloss.Width(line), line)
		}
	}
}

func TestHelpers(t *testing.T) {
	if wordCount("a b c") != 3 {
		t.Error("wordCount")
	}
	if truncate("hello world", 5) != "hell…" {
		t.Errorf("truncate: %q", truncate("hello world", 5))
	}
	if truncate("hi", 0) != "" {
		t.Error("truncate zero width")
	}
	if clamp(5, 0, 3) != 3 || clamp(-1, 0, 3) != 0 || clamp(2, 0, 3) != 2 {
		t.Error("clamp")
	}
	if firstLine("a\nb") != "a" {
		t.Error("firstLine")
	}
	if clock(-time.Second) != "00:00" || clock(65*time.Second) != "01:05" {
		t.Errorf("clock: %q", clock(65*time.Second))
	}
	if previewText("", 5) != "" {
		t.Error("previewText empty")
	}
	if generationPercent("") >= 0.1 {
		t.Error("empty percent should be low")
	}
	if generationPercent("# T <aside Executive Summary\n## a\n## b") < 0.5 {
		t.Error("rich content should score higher")
	}
	_ = focusView(30*time.Second, 40)
	_ = focusView(26*time.Minute, 40) // over-time branch
}

func TestEffectiveModel(t *testing.T) {
	m := newModel(t, 1)
	m.engineName = "claude"
	if got := m.effectiveModel(); got != "sonnet" {
		t.Errorf("claude default = %q", got)
	}
	m.engineName = "ollama"
	if got := m.effectiveModel(); got != "qwen3:4b" {
		t.Errorf("ollama model = %q", got)
	}
	m.cfg.Model = "opus"
	m.engineName = "claude"
	if got := m.effectiveModel(); got != "opus" {
		t.Errorf("override = %q", got)
	}
	m.cfg.Model = ""
	m.engineName = "crush"
	if got := m.effectiveModel(); got != "session default" {
		t.Errorf("crush = %q", got)
	}
}

func TestStartJobOutOfRange(t *testing.T) {
	m := newModel(t, 1)
	if cmd := m.startJob(99); cmd != nil {
		t.Error("out-of-range job index should return nil")
	}
}

func TestPreviewTextTruncates(t *testing.T) {
	in := "l1\nl2\nl3\nl4\nl5"
	if got := previewText(in, 2); got != "l4\nl5" {
		t.Errorf("previewText should keep the last N lines, got %q", got)
	}
}

func TestHandleKeyUpAndPageUp(t *testing.T) {
	m := newModel(t, 1)
	m = upd(m, tea.WindowSizeMsg{Width: 100, Height: 8})
	m.scroll = 50
	m = upd(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = upd(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = upd(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("up")})
	m = upd(m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.scroll >= 50 {
		t.Error("up/pgup should decrease scroll")
	}
}
