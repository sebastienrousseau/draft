// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/internal/pdf"
	"github.com/sebastienrousseau/draft/internal/runes"
)

const (
	maxTemplateChars        = 1000
	maxTemplateFiles        = 2
	maxTemplateExcerptChars = maxTemplateChars / maxTemplateFiles
)

// templateHeadingLine matches a Markdown heading in a user template, dropped from
// the style sample so a literal model has no template headings to copy verbatim.
var templateHeadingLine = regexp.MustCompile(`(?m)^#{1,6} .*$`)

// loadTemplates builds an optional style-calibration block from the user's local
// template directory, if one exists. When absent (as on a fresh checkout), it
// returns "" and the prompt falls back to its built-in style example, so the
// tool works with zero local setup.
func loadTemplates(cfg config.Config) string {
	dirs := []string{
		filepath.Join(cfg.HomeDir, "Drop", "Templates"),
		filepath.Join(cfg.DraftsDir, "Templates"),
	}
	var dir string
	for _, d := range dirs {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			dir = d
			break
		}
	}
	if dir == "" {
		return ""
	}

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	sort.Slice(files, func(i, j int) bool { return modTime(files[i]) > modTime(files[j]) })
	if len(files) > maxTemplateFiles {
		files = files[:maxTemplateFiles]
	}

	var parts []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		// Drop heading lines before excerpting: the style sample is a tone sample,
		// and a literal model would otherwise copy the template's headings into
		// unrelated drafts. Heading structure comes from the OUTPUT SKELETON.
		prose := templateHeadingLine.ReplaceAllString(string(b), "")
		text := pdf.NormaliseSpace(prose)
		if text == "" {
			continue
		}
		excerpt := runes.CutHead(text, maxTemplateExcerptChars)
		// Only the prose excerpt is shown, as a tone sample. A heading outline used
		// to be included, but a literal model copied those headings verbatim into
		// unrelated drafts; heading structure comes from the OUTPUT SKELETON instead.
		parts = append(parts, "## Template example: "+filepath.Base(f)+"\n\n### Style sample\n"+excerpt)
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func modTime(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.ModTime().UnixNano()
}
