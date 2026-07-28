// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/pipeline"
	"github.com/sebastienrousseau/draft/rules"
)

// Version is the build version rendered in the TUI footer, injected by
// cmd/draft; the "dev" fallback makes an un-injected build obvious.
var Version = "dev"

// Palette — mirrors the corral UI language: one coral accent over a quiet
// ramp of grays, green and red reserved for final outcomes.
const (
	coral     = "#F56B5E"
	coralSoft = "#FF8A7A"
)

var (
	accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(coral)).Bold(true)
	titleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(coral)).Bold(true)
	valueStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	subtleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	logStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	mutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	sepStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("239"))
	ruleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
)

// The nib: a fountain-pen point in the same braille art and red-gradient
// treatment as the corral logo.
var logoLines = []string{
	`      ⢀⣀⣀⡀      `,
	`    ⢀⣴⣿⣿⣿⣿⣦⡀    `,
	`   ⢠⣿⣿⣿⠛⣿⣿⣿⡄   `,
	`   ⣾⣿⣿⡇⠐⢸⣿⣿⣷   `,
	`   ⠹⣿⣿⣧⣤⣼⣿⣿⠏   `,
	`    ⠈⠻⣿⣿⣿⠟⠁     `,
	`       ⠙⠋       `,
}

var logoColors = []string{
	"#F87171",
	"#F25447",
	"#D5473D",
	"#C93F36",
	"#B02E28",
	"#A22030",
	"#9F1239",
}

// styledLogo returns the gradient nib, wordmark, and tagline.
func styledLogo() string {
	var sb strings.Builder
	sb.WriteString("\n")
	for i, line := range logoLines {
		sb.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(logoColors[i])).Render(line) + "\n")
	}
	sb.WriteString("\n")
	sb.WriteString("  " + accentStyle.Render("Draft.") + "\n")
	sb.WriteString("  " + valueStyle.Render("From paper to post. Grounded.") + "\n\n")
	return sb.String()
}

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
	b.WriteString("\n")

	if m.allDone {
		b.WriteString(m.renderSummary(contentWidth))
		b.WriteString("\n\n" + m.renderFooter(contentWidth))
		return m.scrollView(b.String())
	}

	headerLines := 4
	if m.showLogo() {
		headerLines = len(logoLines) + 8
	}
	panelHeight := clamp(m.height-headerLines-6, 10, 22)
	leftWidth := clamp(contentWidth/2-2, 34, 52)
	rightWidth := contentWidth - leftWidth - 6
	left := m.renderControlPanel(leftWidth, panelHeight)
	right := m.renderPreviewPanel(rightWidth, panelHeight)
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, "  ", left, "    ", right))
	b.WriteString("\n\n" + shortcutBar([][2]string{
		{"[q]", "quit"}, {"[j/k]", "up/down"}, {"[pgup/pgdn]", "page"},
	}))
	b.WriteString("\n\n" + m.renderFooter(contentWidth))
	return m.scrollView(b.String())
}

// showLogo reports whether the gradient logo header fits this run: tall
// enough terminal, not the closing summary, and not disabled via env.
func (m Model) showLogo() bool {
	return os.Getenv("DRAFT_SHOW_LOGO") != "0" && m.height >= 28
}

// renderHeader draws the corral-style masthead: the gradient logo with an
// activity line when there is room, or a single wordmark line when there is
// not. Both end in a thin divider, and no line may exceed width.
func (m Model) renderHeader(width int) string {
	divider := "  " + ruleStyle.Render(strings.Repeat("─", clamp(width-4, 20, 58)))
	activity := m.activityLine(width - 6)

	if m.showLogo() {
		return styledLogo() + "  " + subtleStyle.Render("⧇ "+activity) + "\n" + divider + "\n"
	}

	line := "  " + accentStyle.Render("Draft.") + "  " + subtleStyle.Render(activity)
	if lipgloss.Width(line) > width {
		line = "  " + accentStyle.Render("Draft.")
	}
	return line + "\n" + divider + "\n"
}

// activityLine is the one-sentence run status, dropping detail from the right
// (word range first, then model, then the label) until it fits budget.
func (m Model) activityLine(budget int) string {
	status := m.engineName + " · offline"
	if m.engineName != "ollama" {
		status = "online · " + m.engineName
	}
	candidates := []string{
		fmt.Sprintf("Drafting Grounded Articles — %s · %s · %d–%d words",
			status, m.effectiveModel(), rules.MinWords, rules.MaxWords),
		fmt.Sprintf("Drafting Grounded Articles — %s · %s", status, m.effectiveModel()),
		fmt.Sprintf("Drafting Grounded Articles — %s", status),
		status,
	}
	for _, c := range candidates {
		if len([]rune(c)) <= budget {
			return c
		}
	}
	return truncate(status, max(1, budget))
}

// effectiveModel is the model label to display for the active engine.
func (m Model) effectiveModel() string {
	if m.engineName == "ollama" {
		return m.cfg.OllamaModel
	}
	if m.cfg.Model != "" {
		return m.cfg.Model
	}
	if p, ok := engine.LookupProvider(m.engineName); ok && p.DefaultModel != "" {
		return p.DefaultModel
	}
	return "session default"
}

// section renders a corral-style section head: coral title over a thin rule.
func section(title string, width int) string {
	return titleStyle.Render(title) + "\n" + ruleStyle.Render(strings.Repeat("─", clamp(width, 8, 58))) + "\n"
}

func (m Model) renderControlPanel(width, height int) string {
	var b strings.Builder
	b.WriteString(section("Queue", width-4))
	for i, res := range m.results {
		b.WriteString(m.queueLine(i, res, width-6))
	}
	b.WriteString("\n")
	b.WriteString(section("Pipeline", width-4))
	for i, name := range pipeline.PhaseNames {
		b.WriteString(m.phaseLine(m.phases[i], name))
	}
	if !m.genStarted.IsZero() && height >= 16 {
		b.WriteString("\n")
		b.WriteString(focusView(time.Since(m.genStarted), width-6))
		b.WriteString("\n")
	}
	if len(m.logs) > 0 {
		b.WriteString("\n")
		b.WriteString(section("Log", width-4))
		logs := m.logs
		if height < 18 && len(logs) > 3 {
			logs = logs[len(logs)-3:]
		}
		for _, entry := range logs {
			b.WriteString(logStyle.Render("· "+truncate(entry, width-8)) + "\n")
		}
	}
	return lipgloss.NewStyle().Width(width).Render(strings.TrimRight(b.String(), "\n"))
}

func (m Model) renderPreviewPanel(width, height int) string {
	var b strings.Builder
	b.WriteString(section("Live Draft", width-4))
	b.WriteString(m.spinner.View() + " " + helpStyle.Render(m.statusText()) + "\n")
	pct := generationPercent(m.output)
	m.progress.Width = clamp(width-13, 12, 48) // leave room for the appended " 100%"
	bar := m.progress.ViewAs(pct)
	b.WriteString(bar + accentStyle.Render(fmt.Sprintf(" %3.0f%%", pct*100)) + "\n\n")
	preview := strings.TrimSpace(m.preview)
	if preview == "" {
		preview = helpStyle.Render("Waiting for the first Markdown lines.")
	}
	body := b.String() + preview
	return lipgloss.NewStyle().Width(width).MaxHeight(height).Render(body)
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
	head := okStyle.Render("✓ Done.")
	if failed > 0 {
		head = errStyle.Render("✗ Finished with failures.")
	}
	head += valueStyle.Render(fmt.Sprintf(" Drafted %d, failed %d.", done, failed))
	b.WriteString("  " + head + "\n\n")
	for _, r := range m.results {
		switch r.state {
		case stateDone:
			b.WriteString("  " + okStyle.Render("✓") + " " + valueStyle.Render(r.label) +
				subtleStyle.Render(fmt.Sprintf("  %d words · %s", r.words, r.engine)) + "\n")
			b.WriteString(helpStyle.Render("     "+r.outputPath) + "\n")
		case stateFailed:
			b.WriteString("  " + errStyle.Render("✗") + " " + valueStyle.Render(r.label) + "\n")
			b.WriteString(helpStyle.Render("     "+firstLine(r.errText)) + "\n")
		}
	}
	b.WriteString("\n  " + titleStyle.Render("Next") + "\n")
	b.WriteString("  " + ruleStyle.Render(strings.Repeat("─", clamp(width-8, 8, 58))) + "\n")
	in := m.input
	if width > 30 {
		in.Width = width - 12
	}
	b.WriteString("  " + in.View() + "\n\n")
	b.WriteString(shortcutBar([][2]string{
		{"[enter]", "queue another"}, {"[q]", "quit"},
	}))
	return b.String()
}

// shortcutBar renders corral-style key hints: coral keys, gray descriptions,
// dotted separators.
func shortcutBar(pairs [][2]string) string {
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, accentStyle.Render(p[0])+" "+helpStyle.Render(p[1]))
	}
	return "  " + strings.Join(parts, sepStyle.Render(" • "))
}

// renderFooter right-aligns the signature line under the whole view.
func (m Model) renderFooter(width int) string {
	left := " " + m.engineFootnote()
	right := fmt.Sprintf("Made with ❤️ in London, UK (v%s)", Version)
	gap := width - lipgloss.Width(left) - len([]rune(right))
	if gap < 2 {
		gap = 2
	}
	return helpStyle.Render(" "+left+strings.Repeat(" ", gap)+right) + "\n"
}

func (m Model) engineFootnote() string {
	if m.engineName == "" {
		return "draft"
	}
	return "draft · " + m.engineName
}

func (m Model) queueLine(i int, res jobResult, width int) string {
	marker := mutedStyle.Render("·")
	name := subtleStyle.Render(truncate(res.label, width))
	switch res.state {
	case stateRunning:
		marker = accentStyle.Render(m.spinner.View())
		name = valueStyle.Render(truncate(res.label, width))
	case stateDone:
		marker = okStyle.Render("✓")
		name = valueStyle.Render(truncate(res.label, width))
	case stateFailed:
		marker = errStyle.Render("✗")
		name = errStyle.Render(truncate(res.label, width))
	}
	counter := mutedStyle.Render(fmt.Sprintf("[%d/%d] ", i+1, len(m.results)))
	return counter + marker + " " + name + "\n"
}

func (m Model) phaseLine(status, name string) string {
	marker := mutedStyle.Render("·")
	label := subtleStyle.Render(name)
	switch status {
	case "running":
		marker = accentStyle.Render(m.spinner.View())
		label = valueStyle.Render(name)
	case "done":
		marker = okStyle.Render("✓")
		label = valueStyle.Render(name)
	case "failed":
		marker = errStyle.Render("✗")
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
	footer := ruleStyle.Render(fmt.Sprintf("  scroll %d/%d", scroll, maxScroll)) +
		sepStyle.Render(" • ") + helpStyle.Render("j/k · arrows · pgup/pgdn · wheel")
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
	bar := accentStyle.Render(strings.Repeat("━", filled)) + ruleStyle.Render(strings.Repeat("─", barWidth-filled))
	return section("Focus", barWidth) + bar + "\n" + valueStyle.Render(line)
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
