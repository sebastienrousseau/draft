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

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sebastienrousseau/draft/internal/config"
	"github.com/sebastienrousseau/draft/internal/engine"
	"github.com/sebastienrousseau/draft/internal/pipeline"
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
	ctx      context.Context
	cfg      config.Config
	primary  engine.Engine
	fallback engine.Engine

	jobs    []pipeline.Job
	results []jobResult
	current int
	events  chan pipeline.Event

	phases     [pipeline.NumPhases]string
	logs       []string
	output     string
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

// New constructs the initial model for a set of jobs.
func New(ctx context.Context, cfg config.Config, primary, fallback engine.Engine, jobs []pipeline.Job) Model {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = accentStyle

	pr := progress.New(progress.WithGradient(cyan, cyanSoft), progress.WithoutPercentage())
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
		cfg:      cfg,
		primary:  primary,
		fallback: fallback,
		jobs:     jobs,
		results:  results,
		events:   make(chan pipeline.Event, 256),
		spinner:  sp,
		progress: pr,
		input:    ti,
		started:  time.Now(),
	}
	m.resetPhases()
	m.engineName = primary.Name()
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
	m.output = ""
	m.preview = ""
	m.words = 0
	m.started = time.Now()
	m.genStarted = time.Time{}
	job := m.jobs[i]
	events := m.events
	runner := pipeline.NewRunner(m.cfg, m.primary, m.fallback, events)
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
		switch msg.Type {
		case tea.MouseWheelDown:
			m.scroll += 3
		case tea.MouseWheelUp:
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
		m.logs = append(m.logs, string(msg))
		if len(m.logs) > 8 {
			m.logs = m.logs[len(m.logs)-8:]
		}
		return m, waitForEvent(m.events)
	case pipeline.TokenEvent:
		m.output += string(msg)
		m.preview = previewText(m.output, 16)
		m.words = wordCount(m.preview)
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
