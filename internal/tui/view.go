package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/sebastienrousseau/draft/internal/config"
	"github.com/sebastienrousseau/draft/internal/pipeline"
	"github.com/sebastienrousseau/draft/internal/rules"
)

// Palette.
const (
	cyan     = "#68FEE3"
	cyanSoft = "#9CFFF1"
	cyanDim  = "#1F5F5B"
	panelDim = "#2A3D3E"
)

var (
	titleStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cyanSoft))
	mutedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	okStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color(cyan)).Bold(true)
	errStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	accentStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(cyan)).Bold(true)
	labelStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color(cyan)).Bold(true)
	valueStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	ruleStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color(cyanDim))
	panelStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(panelDim)).Padding(1, 2)
	activePanelStyle = panelStyle.BorderForeground(lipgloss.Color(cyanDim))
	headerStyle      = lipgloss.NewStyle().Padding(0, 1)
)

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 {
		m.width = 100
	}
	if m.height == 0 {
		m.height = 30
	}
	contentWidth := clamp(m.width-4, 78, 136)

	var b strings.Builder
	b.WriteString(m.renderHeader(contentWidth))
	b.WriteString("\n\n")

	if m.allDone {
		b.WriteString(m.renderSummary(contentWidth))
		return m.scrollView(b.String())
	}

	panelHeight := clamp(m.height-10, 12, 22)
	leftWidth := clamp(contentWidth/2-2, 34, 52)
	rightWidth := contentWidth - leftWidth - 6
	left := m.renderControlPanel(leftWidth, panelHeight)
	right := m.renderPreviewPanel(rightWidth, panelHeight)
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right))
	b.WriteString("\n\n" + mutedStyle.Render("Press q to quit."))
	return m.scrollView(b.String())
}

func (m Model) renderHeader(width int) string {
	mark := accentStyle.Render("ᚲ  DRAFT")
	online := mutedStyle.Render("offline · ollama")
	if m.engineName == "claude" {
		online = accentStyle.Render("online · claude")
	}
	model := m.cfg.OllamaModel
	if m.engineName == "claude" {
		model = m.cfg.ClaudeModel
	}
	left := mark + mutedStyle.Render("  research → grounded markdown")
	right := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(
		fmt.Sprintf("%s   %s   %d–%d words", online, model, rules.MinWords, rules.MaxWords),
	)
	gap := max(2, width-lipgloss.Width(left)-lipgloss.Width(right))
	body := left + strings.Repeat(" ", gap) + right
	return headerStyle.Width(width).Render(body + "\n" + ruleStyle.Render(strings.Repeat("─", max(0, width-2))))
}

func (m Model) renderControlPanel(width, height int) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Queue"))
	b.WriteString("\n")
	for i, res := range m.results {
		b.WriteString(m.queueLine(i, res, width-6))
	}
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("Pipeline"))
	b.WriteString("\n")
	for i, name := range pipeline.PhaseNames {
		b.WriteString(m.phaseLine(m.phases[i], name))
	}
	if !m.genStarted.IsZero() && height >= 18 {
		b.WriteString("\n")
		b.WriteString(focusView(time.Since(m.genStarted), width-6))
	}
	if len(m.logs) > 0 {
		b.WriteString("\n")
		b.WriteString(titleStyle.Render("Log"))
		b.WriteString("\n")
		logs := m.logs
		if height < 18 && len(logs) > 3 {
			logs = logs[len(logs)-3:]
		}
		for _, entry := range logs {
			b.WriteString(mutedStyle.Render("· ") + truncate(entry, width-8) + "\n")
		}
	}
	return activePanelStyle.Width(width).Height(height).Render(strings.TrimRight(b.String(), "\n"))
}

func (m Model) renderPreviewPanel(width, height int) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Live Draft"))
	b.WriteString("\n")
	b.WriteString(m.spinner.View() + " " + mutedStyle.Render(m.statusText()) + "\n")
	b.WriteString(m.progress.ViewAs(generationPercent(m.output)) + "\n\n")
	preview := strings.TrimSpace(m.preview)
	if preview == "" {
		preview = mutedStyle.Render("Waiting for the first Markdown lines.")
	}
	return panelStyle.Width(width).Height(height).Render(b.String() + preview)
}

func (m Model) renderSummary(width int) string {
	var b strings.Builder
	done, failed := 0, 0
	for _, r := range m.results {
		switch r.state {
		case stateDone:
			done++
		case stateFailed:
			failed++
		}
	}
	head := okStyle.Render(fmt.Sprintf("Queue complete — %d drafted", done))
	if failed > 0 {
		head += errStyle.Render(fmt.Sprintf(", %d failed", failed))
	}
	b.WriteString(head + "\n\n")
	for _, r := range m.results {
		switch r.state {
		case stateDone:
			b.WriteString(okStyle.Render("✓ ") + valueStyle.Render(r.label) +
				mutedStyle.Render(fmt.Sprintf("  %d words · %s", r.words, r.engine)) + "\n")
			b.WriteString(mutedStyle.Render("   "+r.outputPath) + "\n")
		case stateFailed:
			b.WriteString(errStyle.Render("× ") + valueStyle.Render(r.label) + "\n")
			b.WriteString(mutedStyle.Render("   "+firstLine(r.errText)) + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Next") + "\n")
	in := m.input
	if width > 30 {
		in.Width = width - 12
	}
	b.WriteString(in.View() + "\n")
	b.WriteString(mutedStyle.Render("Enter queues another source. q or esc closes the session."))
	return activePanelStyle.Width(width).Render(b.String())
}

func (m Model) queueLine(i int, res jobResult, width int) string {
	marker := mutedStyle.Render("○")
	name := mutedStyle.Render(truncate(res.label, width))
	switch res.state {
	case stateRunning:
		marker = accentStyle.Render(m.spinner.View())
		name = valueStyle.Render(truncate(res.label, width))
	case stateDone:
		marker = okStyle.Render("✓")
		name = valueStyle.Render(truncate(res.label, width))
	case stateFailed:
		marker = errStyle.Render("×")
		name = errStyle.Render(truncate(res.label, width))
	}
	counter := mutedStyle.Render(fmt.Sprintf("[%d/%d] ", i+1, len(m.results)))
	return counter + marker + " " + name + "\n"
}

func (m Model) phaseLine(status, name string) string {
	marker := mutedStyle.Render("○")
	label := mutedStyle.Render(name)
	switch status {
	case "running":
		marker = accentStyle.Render(m.spinner.View())
		label = valueStyle.Render(name)
	case "done":
		marker = okStyle.Render("✓")
		label = valueStyle.Render(name)
	case "failed":
		marker = errStyle.Render("×")
		label = errStyle.Render(name)
	}
	return marker + " " + label + "\n"
}

func (m Model) statusText() string {
	for i, s := range m.phases {
		if s == "running" {
			switch i {
			case pipeline.PhaseWrite:
				if m.words > 0 {
					return fmt.Sprintf("writing, %d words visible", m.words)
				}
				return "waiting for tokens"
			default:
				return strings.ToLower(pipeline.PhaseNames[i])
			}
		}
	}
	return "queued"
}

func (m Model) scrollView(s string) string {
	if m.height <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	visible := max(1, m.height-1)
	maxScroll := max(0, len(lines)-visible)
	scroll := clamp(m.scroll, 0, maxScroll)
	if maxScroll == 0 {
		return strings.Join(lines, "\n")
	}
	end := min(len(lines), scroll+visible)
	footer := ruleStyle.Render(fmt.Sprintf("scroll %d/%d  j/k · arrows · pgup/pgdn · wheel", scroll, maxScroll))
	return strings.Join(lines[scroll:end], "\n") + "\n" + footer
}

func focusView(elapsed time.Duration, width int) string {
	elapsed = elapsed.Round(time.Second)
	remaining := config.FocusBlock - elapsed
	var line string
	if remaining < 0 {
		line = fmt.Sprintf("%s over, stand up", clock((-remaining).Round(time.Second)))
	} else {
		line = fmt.Sprintf("%s / 25:00", clock(elapsed))
	}
	barWidth := clamp(width-6, 16, 32)
	pct := float64(elapsed) / float64(config.FocusBlock)
	if pct > 1 {
		pct = 1
	}
	filled := int(pct * float64(barWidth))
	bar := strings.Repeat("━", filled) + strings.Repeat("─", barWidth-filled)
	return titleStyle.Render("Focus timer") + "\n" + accentStyle.Render(bar) + "\n" + valueStyle.Render(line)
}

func generationPercent(s string) float64 {
	if strings.TrimSpace(s) == "" {
		return 0.05
	}
	score := 0.18
	if strings.Contains(s, "# ") {
		score = 0.30
	}
	if strings.Contains(s, "<aside") {
		score = 0.46
	}
	if strings.Contains(s, "Executive Summary") {
		score = 0.62
	}
	sections := strings.Count(s, "\n## ")
	score += float64(min(sections, 4)) * 0.08
	if score > 0.94 {
		score = 0.94
	}
	return score
}

func clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

func truncate(s string, width int) string {
	if width <= 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
