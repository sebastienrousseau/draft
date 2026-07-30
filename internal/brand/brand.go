// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package brand holds draft's visual identity — the logo, the palette, and
// the shared lipgloss styles — so the command-line surface and the dashboard
// present the same face.
package brand

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette: one coral accent over a quiet ramp of grays, with green and red
// reserved for outcomes.
const (
	Coral     = "#F56B5E"
	CoralSoft = "#FF8A7A"
)

// Shared styles.
var (
	Accent = lipgloss.NewStyle().Foreground(lipgloss.Color(Coral)).Bold(true)
	Title  = lipgloss.NewStyle().Foreground(lipgloss.Color(Coral)).Bold(true)
	Value  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	Subtle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	Help   = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	Log    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	Muted  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	Sep    = lipgloss.NewStyle().Foreground(lipgloss.Color("239"))
	Rule   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	OK     = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	Err    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
)

// Wordmark is the name as it is set everywhere draft presents itself —
// lowercase, no full stop, matching the command you actually type and the
// wordmark on draftlib.com. It lives here as a constant because it was once
// copied as a literal into four files and drifted out of step with the site.
const Wordmark = "draft"

// Tagline is the one-line description shown under the wordmark.
const Tagline = "From paper to post. Grounded."

// LogoLines is the nib: a fountain-pen point — shoulders, vent hole, slit,
// and tip — drawn in braille so it scales with the terminal font.
var LogoLines = []string{
	`   ⣰⣿⣿⣿⣿⣿⣿⣆   `,
	`   ⣿⣿⣿⣿⣿⣿⣿⣿   `,
	`   ⣿⣿⣿⠿⠿⣿⣿⣿   `,
	`   ⠹⣿⣿⡇⢸⣿⣿⠏   `,
	`    ⠻⣿⡇⢸⣿⠟    `,
	`     ⠹⡇⢸⠏     `,
	`      ⠘⠃      `,
}

// LogoColors is the red gradient applied down the nib, one entry per line.
var LogoColors = []string{
	"#F87171",
	"#F25447",
	"#D5473D",
	"#C93F36",
	"#B02E28",
	"#A22030",
	"#9F1239",
}

// ShowLogo reports whether the logo may be drawn at all. DRAFT_SHOW_LOGO=0
// opts out everywhere the logo appears.
func ShowLogo() bool { return os.Getenv("DRAFT_SHOW_LOGO") != "0" }

// Logo returns the gradient nib with the wordmark and tagline. When compact,
// the tagline sits beside the wordmark and the surrounding blank lines are
// dropped, so the block still fits a standard 24-row terminal.
func Logo(compact bool) string {
	var sb strings.Builder
	if !compact {
		sb.WriteString("\n")
	}
	for i, line := range LogoLines {
		sb.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(LogoColors[i])).Render(line) + "\n")
	}
	sb.WriteString("\n")
	if compact {
		sb.WriteString("  " + Accent.Render(Wordmark) + "  " + Subtle.Render(Tagline) + "\n")
		return sb.String()
	}
	sb.WriteString("  " + Accent.Render(Wordmark) + "\n")
	sb.WriteString("  " + Value.Render(Tagline) + "\n\n")
	return sb.String()
}
