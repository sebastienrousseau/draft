// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/brand"
	"github.com/sebastienrousseau/draft/pipeline"
	"github.com/sebastienrousseau/draft/rules"
)

// Version is the build version rendered in the TUI footer, injected by
// cmd/draft; the "dev" fallback makes an un-injected build obvious.
var Version = "dev"

// Palette, logo, and styles come from the shared brand package so the
// dashboard and the command-line surface present the same face.
var (
	accentStyle = brand.Accent
	titleStyle  = brand.Title
	valueStyle  = brand.Value
	subtleStyle = brand.Subtle
	helpStyle   = brand.Help
	logStyle    = brand.Log
	mutedStyle  = brand.Muted
	sepStyle    = brand.Sep
	ruleStyle   = brand.Rule
	okStyle     = brand.OK
	errStyle    = brand.Err

	logoLines = brand.LogoLines
)

const (
	coral     = brand.Coral
	coralSoft = brand.CoralSoft
)

// styledLogo renders the brand logo block.
func styledLogo(compact bool) string { return brand.Logo(compact) }

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

	if m.selectingEngine {
		return m.renderEngineSelectView(contentWidth)
	}

	if m.allDone {
		b.WriteString(m.renderSummary(contentWidth))
		b.WriteString("\n\n" + m.renderFooter(contentWidth))
		return m.scrollView(b.String())
	}

	// Budget the panels against everything else on screen: the header, the
	// blank line under it, the chrome below (shortcut bar and footer, each
	// preceded by a blank unless tight), and the row scrollView reserves.
	chromeHeight := 4
	if m.tight() {
		chromeHeight = 2
	}
	panelHeight := clamp(m.height-m.headerHeight()-chromeHeight-2, 6, 22)
	leftWidth := clamp(contentWidth/2-2, 34, 52)
	rightWidth := contentWidth - leftWidth - 6
	left := m.renderControlPanel(leftWidth, panelHeight)
	right := m.renderPreviewPanel(rightWidth, panelHeight)
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, "  ", left, "    ", right))
	chrome := "\n\n"
	if m.tight() {
		chrome = "\n"
	}
	b.WriteString(chrome + shortcutBar([][2]string{
		{"[q]", "quit"}, {"[j/k]", "up/down"}, {"[pgup/pgdn]", "page"},
	}))
	b.WriteString(chrome + m.renderFooter(contentWidth))
	return m.scrollView(b.String())
}

// showLogo reports whether the gradient logo header fits this run. The block
// costs 11 rows, so a standard 24-row terminal is exactly the floor at which
// it can appear without crowding out the queue and pipeline; anything shorter
// falls back to the one-line masthead. DRAFT_SHOW_LOGO=0 opts out entirely.
func (m Model) showLogo() bool {
	return brand.ShowLogo() && m.height >= 24
}

// compactLogo reports whether the logo block drops its tagline line and
// surrounding padding to save vertical space.
func (m Model) compactLogo() bool { return m.height < 32 }

// headerHeight is the number of lines renderHeader emits, so the panels can
// be sized to fill exactly the rest of the terminal.
func (m Model) headerHeight() int {
	if !m.showLogo() {
		return 2
	}
	if m.compactLogo() {
		return len(logoLines) + 4
	}
	return len(logoLines) + 7
}

// renderHeader draws the corral-style masthead: the gradient logo with an
// activity line when there is room, or a single wordmark line when there is
// not. Both end in a thin divider, and no line may exceed width.
func (m Model) renderHeader(width int) string {
	divider := "  " + ruleStyle.Render(strings.Repeat("─", clamp(width-4, 20, 58)))
	activity := m.activityLine(width - 6)

	if m.showLogo() {
		return styledLogo(m.compactLogo()) + "  " + subtleStyle.Render("⧇ "+activity) + "\n" + divider + "\n"
	}

	line := "  " + accentStyle.Render(brand.Wordmark) + "  " + subtleStyle.Render(activity)
	if lipgloss.Width(line) > width {
		line = "  " + accentStyle.Render(brand.Wordmark)
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

// tight reports whether the terminal is short enough that decoration — section
// rules and blank separators — must give way to information.
func (m Model) tight() bool { return m.height < 30 }

// section renders a corral-style section head: coral title over a thin rule.
// In tight mode the rule is dropped, keeping the title alone.
func (m Model) section(title string, width int) string {
	if m.tight() {
		return titleStyle.Render(title) + "\n"
	}
	return titleStyle.Render(title) + "\n" + ruleStyle.Render(strings.Repeat("─", clamp(width, 8, 58))) + "\n"
}

func (m Model) renderControlPanel(width, height int) string {
	gap := "\n"
	if m.tight() {
		gap = ""
	}

	var b strings.Builder
	b.WriteString(m.section("Queue", width-4))
	for i, res := range m.results {
		b.WriteString(m.queueLine(i, res, width-6))
	}
	b.WriteString(gap)
	b.WriteString(m.section("Pipeline", width-4))
	for i, name := range pipeline.PhaseNames {
		b.WriteString(m.phaseLine(m.phases[i], name))
	}
	// Spend the remaining lines on the focus timer and the log, in that
	// order, and only when they genuinely fit — a half-drawn section is
	// worse than none.
	used := lineCount(b.String())
	if !m.genStarted.IsZero() && height-used >= 4 {
		b.WriteString(gap)
		b.WriteString(m.focusView(time.Since(m.genStarted), width-6))
		b.WriteString("\n")
		used = lineCount(b.String())
	}
	if room := height - used - 1; len(m.logs) > 0 && room >= 2 {
		b.WriteString(gap)
		b.WriteString(m.section("Log", width-4))
		logs := m.logs
		if len(logs) > room-1 {
			logs = logs[len(logs)-(room-1):]
		}
		for _, entry := range logs {
			b.WriteString(logStyle.Render("· "+truncate(entry, width-8)) + "\n")
		}
	}
	return lipgloss.NewStyle().Width(width).MaxHeight(height).Render(strings.TrimRight(b.String(), "\n"))
}

// lineCount counts rendered lines in s, ignoring a single trailing newline.
func lineCount(s string) int {
	return len(strings.Split(strings.TrimRight(s, "\n"), "\n"))
}

func (m Model) renderPreviewPanel(width, height int) string {
	var b strings.Builder
	b.WriteString(m.section("Live Draft", width-4))
	b.WriteString(m.spinner.View() + " " + helpStyle.Render(m.statusText()) + "\n")
	pct := generationPercent(m.article())
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
	return helpStyle.Render(" " + left + strings.Repeat(" ", gap) + right)
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

func (m Model) focusView(elapsed time.Duration, width int) string {
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
	return m.section("Focus", barWidth) + bar + "\n" + valueStyle.Render(line)
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

func (m Model) renderEngineSelectView(width int) string {
	var b strings.Builder
	b.WriteString(m.renderHeader(width))
	b.WriteString("\n")

	online := engine.IsOnline()
	statusHeader := "Select LLM Provider / Model Engine:"
	if !online {
		statusHeader += "  " + errStyle.Render("[Offline - Zero Network]")
	} else {
		statusHeader += "  " + okStyle.Render("[Online]")
	}
	b.WriteString("  " + titleStyle.Render(statusHeader) + "\n\n")

	for i, choice := range m.engineChoices {
		prefix := "    "
		style := subtleStyle
		if i == m.engineCursor {
			prefix = "  › "
			style = accentStyle
		}

		var statusBadge string
		switch {
		case choice.Name == "auto":
			if !online {
				statusBadge = okStyle.Render("[auto -> ollama]")
			} else {
				statusBadge = okStyle.Render("[auto]")
			}
		case choice.Name == "ollama":
			if engine.IsOllamaRunning(m.cfg.OllamaHost) {
				statusBadge = okStyle.Render("[running / ready]")
			} else if choice.Installed {
				statusBadge = subtleStyle.Render("[local / auto-start]")
			} else {
				statusBadge = mutedStyle.Render("[not installed]")
			}
		case choice.Installed:
			if !online {
				statusBadge = mutedStyle.Render("[cloud - offline]")
			} else {
				statusBadge = okStyle.Render("[installed]")
			}
		default:
			statusBadge = mutedStyle.Render("[not found]")
		}

		nameCol := style.Render(fmt.Sprintf("%-14s", choice.Name))
		descCol := subtleStyle.Render(choice.Description)

		fmt.Fprintf(&b, "%s%s  %s  %s\n", prefix, nameCol, descCol, statusBadge)
	}

	b.WriteString("\n  " + helpStyle.Render("Press ") + accentStyle.Render("↑/↓") + helpStyle.Render(" or ") + accentStyle.Render("j/k") + helpStyle.Render(" to select an engine, then press ") + accentStyle.Render("Enter") + helpStyle.Render(" to begin.") + "\n")

	chrome := "\n\n"
	if m.tight() {
		chrome = "\n"
	}
	b.WriteString(chrome + shortcutBar([][2]string{
		{"[q]", "quit"}, {"[↑/↓]", "select LLM"}, {"[enter]", "confirm & start"},
	}))
	b.WriteString(chrome + m.renderFooter(width))
	return m.scrollView(b.String())
}
