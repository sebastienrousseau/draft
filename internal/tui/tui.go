// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package tui is the Bubble Tea front end. It renders a full-screen dashboard,
// processes a queue of drafting jobs one at a time, and streams each article as
// it is written. All generation work happens in the pipeline package; this
// package only reflects its Event stream and collects user input for the queue.
package tui

import (
	"context"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/pipeline"
)

type jobState int

const (
	stateQueued jobState = iota
	stateRunning
	stateDone
	stateFailed
)

type jobResult struct {
	label      string
	state      jobState
	outputPath string
	words      int
	engine     string
	errText    string
}

// Model is the Bubble Tea model backing the dashboard.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    config.Config
	runner *pipeline.Runner

	jobs    []pipeline.Job
	results []jobResult
	current int
	events  chan pipeline.Event

	phases [pipeline.NumPhases]string
	logs   []string
	// out accumulates the streamed article; tail is a bounded window over its
	// end. Concatenating into a string and re-splitting the whole article on
	// every streamed chunk made the cost of a token grow with the length of
	// the article already written.
	//
	// These are []byte and not strings.Builder deliberately: Bubble Tea passes
	// Model by value, and a Builder panics when it is copied after first use.
	out        []byte
	tail       []byte
	preview    string
	words      int
	engineName string
	started    time.Time
	genStarted time.Time
	allDone    bool

	width, height int
	spinner       spinner.Model
	progress      progress.Model
	input         textinput.Model
	scroll        int
}

// New constructs the initial model for a set of jobs. cancel, when non-nil, is
// invoked on quit to stop any in-flight pipeline work.
func New(ctx context.Context, cancel context.CancelFunc, cfg config.Config, runner *pipeline.Runner, jobs []pipeline.Job) Model {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = accentStyle

	pr := progress.New(progress.WithGradient(coral, coralSoft), progress.WithoutPercentage())
	pr.Width = 44

	ti := textinput.New()
	ti.Placeholder = "type another source path, then press enter"
	ti.Prompt = accentStyle.Render("› ")
	ti.CharLimit = 500
	ti.Width = 70

	results := make([]jobResult, len(jobs))
	for i, j := range jobs {
		results[i] = jobResult{label: label(j), state: stateQueued}
	}

	m := Model{
		ctx:      ctx,
		cancel:   cancel,
		cfg:      cfg,
		runner:   runner,
		jobs:     jobs,
		results:  results,
		events:   make(chan pipeline.Event, 256),
		spinner:  sp,
		progress: pr,
		input:    ti,
		started:  time.Now(),
	}
	m.resetPhases()
	if runner != nil {
		m.engineName = runner.EngineFor(engine.KindWrite)
	}
	// Init cannot return a mutated model, so reflect the first job's running
	// state here; startJob still launches its goroutine from Init.
	if len(results) > 0 {
		m.results[0].state = stateRunning
	}
	return m
}

// Init starts the first job.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.startJob(0), waitForEvent(m.events), m.spinner.Tick, progressTick())
}

func (m *Model) startJob(i int) tea.Cmd {
	if i >= len(m.jobs) {
		return nil
	}
	m.current = i
	m.results[i].state = stateRunning
	m.resetPhases()
	m.logs = nil
	m.resetOutput()
	m.started = time.Now()
	m.genStarted = time.Time{}
	job := m.jobs[i]
	runner := m.runner
	runner.SetEvents(m.events)
	return func() tea.Msg {
		go runner.Run(m.ctx, job)
		return nil
	}
}

func (m *Model) resetPhases() {
	for i := range m.phases {
		m.phases[i] = "queued"
	}
}

// previewLines is how many lines the live preview shows, and so how many the
// tail needs to keep. previewTailBytes is a backstop for output that arrives
// with few or no line breaks, where there is nothing to trim on.
const (
	previewLines     = 16
	previewTailBytes = 8 << 10
)

// appendToken accumulates a streamed chunk and refreshes the preview.
//
// Its cost is bounded by the preview window rather than by the length of the
// article already written, which matters because it runs once per streamed
// chunk — thousands of times per draft. Appending to a buffer and previewing
// from a bounded tail replaced a whole-article copy plus a whole-article line
// split per chunk: ~76 MB of allocation for one article's stream became ~23 MB,
// and the time halved (see BenchmarkTokenAppend).
func (m *Model) appendToken(chunk string) {
	m.out = append(m.out, chunk...)
	m.tail = append(m.tail, chunk...)
	m.trimTail()
	m.preview = previewText(string(m.tail), previewLines)
	m.words = wordCount(m.preview)
}

// trimTail keeps the tail to the last previewLines lines. Bounding by lines
// rather than by bytes is what keeps the per-chunk cost genuinely constant:
// the preview can only ever show that many lines, so anything older is copied
// and re-scanned for nothing.
func (m *Model) trimTail() {
	// Walk back to the newline that opens the oldest line still visible.
	seen := 0
	for i := len(m.tail) - 1; i >= 0; i-- {
		if m.tail[i] != '\n' {
			continue
		}
		seen++
		if seen > previewLines {
			m.tail = append(m.tail[:0], m.tail[i+1:]...) // copy handles the overlap
			return
		}
	}
	// Backstop for output with few or no line breaks, where the scan above
	// finds nothing to trim on.
	if len(m.tail) > previewTailBytes {
		trimmed := m.tail[len(m.tail)-previewTailBytes:]
		// Never open the preview mid-rune.
		for len(trimmed) > 0 && !utf8.RuneStart(trimmed[0]) {
			trimmed = trimmed[1:]
		}
		m.tail = append(m.tail[:0], trimmed...)
	}
}

// maxLogLines is how many progress lines the log pane keeps.
const maxLogLines = 8

// appendLog adds a progress line, keeping only the most recent ones.
func (m *Model) appendLog(line string) {
	m.logs = append(m.logs, line)
	if len(m.logs) > maxLogLines {
		m.logs = m.logs[len(m.logs)-maxLogLines:]
	}
}

// article returns everything streamed so far.
func (m Model) article() string { return string(m.out) }

// resetOutput clears the accumulated article between jobs. The buffers are
// re-sliced rather than dropped so the next job reuses the capacity.
func (m *Model) resetOutput() {
	m.out = m.out[:0]
	m.tail = m.tail[:0]
	m.preview = ""
	m.words = 0
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if msg.Width > 24 {
			m.progress.Width = min(56, msg.Width-24)
		}
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelDown:
			m.scroll += 3
		case tea.MouseButtonWheelUp:
			m.scroll = max(0, m.scroll-3)
		}
		return m, nil
	case pipeline.PhaseEvent:
		if msg.Index >= 0 && msg.Index < len(m.phases) {
			m.phases[msg.Index] = msg.Status
			if msg.Index == pipeline.PhaseWrite && msg.Status == "running" {
				m.genStarted = time.Now()
			}
		}
		return m, waitForEvent(m.events)
	case pipeline.LogEvent:
		m.appendLog(string(msg))
		return m, waitForEvent(m.events)
	case pipeline.WarnEvent:
		// Marked so a fallback or a failed save is visibly different from
		// ordinary progress in the log pane.
		m.appendLog("! " + string(msg))
		return m, waitForEvent(m.events)
	case pipeline.TokenEvent:
		m.appendToken(string(msg))
		return m, waitForEvent(m.events)
	case pipeline.EngineEvent:
		m.engineName = string(msg)
		return m, waitForEvent(m.events)
	case pipeline.DoneEvent:
		return m.finishJob(jobResult{state: stateDone, outputPath: msg.OutputPath, words: msg.Words, engine: msg.Engine})
	case pipeline.ErrEvent:
		return m.finishJob(jobResult{state: stateFailed, errText: string(msg)})
	case progress.FrameMsg:
		pm, cmd := m.progress.Update(msg)
		m.progress = pm.(progress.Model)
		if !m.allDone {
			return m, progressTick()
		}
		return m, cmd
	case spinner.TickMsg:
		sp, cmd := m.spinner.Update(msg)
		m.spinner = sp
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		if m.cancel != nil {
			m.cancel() // stop any in-flight session/Ollama call
		}
		return m, tea.Quit
	case "enter":
		if m.allDone {
			next := strings.Trim(strings.TrimSpace(m.input.Value()), `"'`)
			if next == "" {
				return m, nil
			}
			m.input.Reset()
			m.input.Blur()
			m.jobs = append(m.jobs, pipeline.Job{Sources: []string{next}})
			m.results = append(m.results, jobResult{label: filepath.Base(next), state: stateQueued})
			m.allDone = false
			cmd := m.startJob(len(m.jobs) - 1)
			return m, tea.Batch(cmd, waitForEvent(m.events), progressTick())
		}
	case "down", "j":
		m.scroll++
		return m, nil
	case "up", "k":
		m.scroll = max(0, m.scroll-1)
		return m, nil
	case "pgdown", " ":
		m.scroll += max(1, m.height-4)
		return m, nil
	case "pgup", "b":
		m.scroll = max(0, m.scroll-max(1, m.height-4))
		return m, nil
	case "home":
		m.scroll = 0
		return m, nil
	case "end":
		m.scroll += 10000
		return m, nil
	}
	if m.allDone {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) finishJob(res jobResult) (tea.Model, tea.Cmd) {
	res.label = m.results[m.current].label
	if res.engine == "" {
		res.engine = m.engineName
	}
	m.results[m.current] = res
	next := m.current + 1
	if next < len(m.jobs) {
		cmd := m.startJob(next)
		return m, tea.Batch(cmd, waitForEvent(m.events), progressTick())
	}
	m.allDone = true
	m.input.Focus()
	return m, nil
}

func waitForEvent(ch <-chan pipeline.Event) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

func progressTick() tea.Cmd {
	return tea.Tick(time.Second/8, func(time.Time) tea.Msg { return progress.FrameMsg{} })
}

func label(j pipeline.Job) string {
	names := make([]string, 0, len(j.Sources))
	for _, s := range j.Sources {
		names = append(names, filepath.Base(s))
	}
	return strings.Join(names, ", ")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func wordCount(s string) int {
	return len(strings.Fields(s))
}

func previewText(s string, lines int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	all := strings.Split(s, "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}
