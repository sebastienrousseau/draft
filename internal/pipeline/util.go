// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sebastienrousseau/draft/internal/config"
)

var (
	ansiEscape = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	titlePat   = regexp.MustCompile(`(?m)^#\s+(.+)$`)
)

// cleanOutput strips ANSI escapes, carriage returns, and control characters
// that a backend might emit around the Markdown.
func cleanOutput(s string) string {
	s = ansiEscape.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r", "")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 32 {
			return r
		}
		return -1
	}, s)
}

// stripThinking removes any chain-of-thought preamble and returns the Markdown
// starting at the first H1.
func stripThinking(s string) string {
	if idx := strings.LastIndex(s, "</think>"); idx >= 0 {
		s = s[idx+len("</think>"):]
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "# ") {
			return strings.TrimSpace(strings.Join(lines[i:], "\n"))
		}
	}
	return strings.TrimSpace(s)
}

func extractTitle(s string) string {
	if m := titlePat.FindStringSubmatch(s); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return "draft-article"
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	out = slugRepeat.ReplaceAllString(out, "-")
	if len(out) > 90 {
		out = strings.Trim(out[:90], "-")
	}
	if out == "" {
		return "draft-article"
	}
	return out
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func shortPath(cfg config.Config, path string) string {
	if cfg.HomeDir != "" && strings.HasPrefix(path, cfg.HomeDir) {
		return "~" + strings.TrimPrefix(path, cfg.HomeDir)
	}
	return path
}
