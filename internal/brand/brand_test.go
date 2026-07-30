// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package brand

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// Logo indexes LogoColors by line, so a mismatch panics at render time.
func TestLogoLinesAndColorsAlign(t *testing.T) {
	if len(LogoLines) != len(LogoColors) {
		t.Fatalf("%d logo lines but %d colors — Logo would panic", len(LogoLines), len(LogoColors))
	}
	if len(LogoLines) == 0 {
		t.Fatal("logo has no lines")
	}
}

// Every line must be the same display width or the nib renders lopsided.
func TestLogoLinesShareWidth(t *testing.T) {
	want := lipgloss.Width(LogoLines[0])
	for i, line := range LogoLines {
		if got := lipgloss.Width(line); got != want {
			t.Errorf("logo line %d is %d cells wide, want %d: %q", i, got, want, line)
		}
		if !utf8.ValidString(line) {
			t.Errorf("logo line %d is not valid UTF-8", i)
		}
	}
}

func TestLogoRendersBothForms(t *testing.T) {
	full, compact := Logo(false), Logo(true)
	for name, out := range map[string]string{"full": full, "compact": compact} {
		if !strings.Contains(out, Wordmark) {
			t.Errorf("%s logo is missing the wordmark", name)
		}
		if !strings.Contains(out, Tagline) {
			t.Errorf("%s logo is missing the tagline", name)
		}
		if !strings.Contains(out, LogoLines[0]) {
			t.Errorf("%s logo is missing the artwork", name)
		}
	}
	// The compact form exists to save vertical space.
	if fullLines, compactLines := lineCount(full), lineCount(compact); compactLines >= fullLines {
		t.Errorf("compact logo is %d lines, not shorter than full at %d", compactLines, fullLines)
	}
}

// The wordmark is set the same way in the terminal, in the dashboard, and on
// draftlib.com: lowercase, no full stop, identical to the command you type.
// It read "Draft." here for a while and no longer matched the site, so the
// shape is pinned rather than left to whoever edits the constant next.
func TestWordmarkIsSetLikeTheCommandName(t *testing.T) {
	if Wordmark != strings.ToLower(Wordmark) {
		t.Errorf("wordmark %q is not lowercase; the site sets it lowercase", Wordmark)
	}
	if strings.ContainsAny(Wordmark, ". ") {
		t.Errorf("wordmark %q carries punctuation or spacing the site does not", Wordmark)
	}
}

func TestShowLogoRespectsEnv(t *testing.T) {
	t.Setenv("DRAFT_SHOW_LOGO", "0")
	if ShowLogo() {
		t.Error("DRAFT_SHOW_LOGO=0 should disable the logo")
	}
	t.Setenv("DRAFT_SHOW_LOGO", "1")
	if !ShowLogo() {
		t.Error("DRAFT_SHOW_LOGO=1 should enable the logo")
	}
	t.Setenv("DRAFT_SHOW_LOGO", "")
	if !ShowLogo() {
		t.Error("an unset value should default to showing the logo")
	}
}

func TestStylesRenderTheirText(t *testing.T) {
	// Styles must pass their content through, whatever the colour profile.
	for name, style := range map[string]lipgloss.Style{
		"accent": Accent, "title": Title, "value": Value, "subtle": Subtle,
		"help": Help, "log": Log, "muted": Muted, "sep": Sep, "rule": Rule,
		"ok": OK, "err": Err,
	} {
		if got := style.Render("draft"); !strings.Contains(got, "draft") {
			t.Errorf("%s style dropped its content: %q", name, got)
		}
	}
}

func lineCount(s string) int { return len(strings.Split(strings.TrimRight(s, "\n"), "\n")) }
