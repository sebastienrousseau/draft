// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package pdf turns a research source file into normalised plain text and
// splits that text into bounded sections suitable for claim extraction.
//
// PDF text is obtained by shelling out to `pdftotext -layout` (Poppler); DOCX
// via macOS `textutil`. Both are documented runtime dependencies rather than
// cgo bindings, which keeps the binary portable and the extraction quality on
// par with the surrounding tooling.
package pdf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// MaxSectionChars bounds a single extraction section so a small local model is
// never asked to hold more than it can attend to at once.
const MaxSectionChars = 4500

var (
	multiBlank  = regexp.MustCompile(`\n{4,}`)
	stopSection = regexp.MustCompile(`(?im)^\s*(?:#{1,6}\s+)?(?:references|bibliography|acknowledge?ments|appendix)\b`)
	sectionHead = regexp.MustCompile(`\n(?:(?:Abstract|Introduction|Related Work|Method|Methods|Experiments|Results|Discussion|Limitations|Conclusion|References)\b)`)
)

// Extract returns the normalised plain text of a .pdf, .docx, .md, or .txt
// file. Unknown suffixes yield an error so the caller can skip them cleanly.
func Extract(ctx context.Context, path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".txt":
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return NormaliseSpace(string(b)), nil
	case ".pdf":
		// Reading order, not physical layout. `-layout` preserves the visual
		// arrangement, which on a two-column paper splices the left and right
		// columns onto shared lines — sentences then break mid-thought and
		// merge with unrelated text, so a claim's SOURCE_QUOTE can never match
		// the source verbatim. Poppler's default flows each column in turn.
		out, err := runTool(ctx, 120*time.Second, "pdftotext", path, "-")
		if err != nil {
			return "", fmt.Errorf("pdftotext: %w", err)
		}
		text := NormaliseSpace(out)
		if text == "" {
			return "", ErrNoTextLayer
		}
		return text, nil
	case ".docx":
		// textutil is macOS-only; elsewhere DOCX is unsupported rather than a
		// confusing "command not found".
		if runtime.GOOS != "darwin" {
			return "", fmt.Errorf("DOCX extraction requires macOS (textutil); convert to PDF or Markdown first")
		}
		out, err := runTool(ctx, 120*time.Second, "textutil", "-convert", "txt", "-stdout", path)
		if err != nil {
			return "", fmt.Errorf("textutil: %w", err)
		}
		return NormaliseSpace(out), nil
	default:
		return "", fmt.Errorf("unsupported source type %q", filepath.Ext(path))
	}
}

// ErrNoTextLayer reports a PDF that carries no extractable text — typically a
// scan or an export of page images. Nothing downstream can ground a claim in
// it, so the run stops here with an explanation rather than failing later with
// an empty ledger.
var ErrNoTextLayer = errors.New(
	"no text could be extracted; the PDF appears to be a scan or image export. " +
		"Run it through OCR first (for example: ocrmypdf in.pdf out.pdf), or supply a text or Markdown source")

// Section is a labelled slice of source text.
type Section struct {
	Label string
	Body  string
}

// SplitSections drops trailing reference/appendix matter, splits on well-known
// paper headings, and hard-caps each chunk at MaxSectionChars, breaking on the
// nearest paragraph or sentence boundary.
func SplitSections(name, text string) []Section {
	text = NormaliseSpace(text)
	if text == "" {
		return nil
	}
	if cut := lastStopIndex(text); cut >= 0 {
		text = strings.TrimSpace(text[:cut])
	}
	if text == "" {
		return nil
	}

	var sections []Section
	idx := 0
	for _, part := range splitKeepHeadings(text) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		for len(part) > MaxSectionChars {
			cut := lastIndexBefore(part, "\n\n", MaxSectionChars)
			if cut < MaxSectionChars/2 {
				cut = lastIndexBefore(part, ". ", MaxSectionChars)
			}
			if cut < MaxSectionChars/2 {
				cut = MaxSectionChars
			}
			idx++
			sections = append(sections, Section{Label: fmt.Sprintf("%s section %d", name, idx), Body: strings.TrimSpace(part[:cut])})
			part = strings.TrimSpace(part[cut:])
		}
		if part != "" {
			idx++
			sections = append(sections, Section{Label: fmt.Sprintf("%s section %d", name, idx), Body: part})
		}
	}
	return sections
}

// lastStopIndex reports where the trailing reference or appendix matter
// begins, or -1 if there is none. It takes the LAST such heading rather than
// the first, because a contents list names "References" in the front matter
// and cutting there discards the entire paper. Taking the last match also
// keeps the degenerate case right: a document that is nothing but a
// bibliography has its single heading at the start, so everything is cut.
func lastStopIndex(text string) int {
	locs := stopSection.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return -1
	}
	return locs[len(locs)-1][0]
}

// NormaliseSpace canonicalises line endings and collapses runs of blank lines.
func NormaliseSpace(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = multiBlank.ReplaceAllString(text, "\n\n\n")
	return strings.TrimSpace(text)
}

// splitKeepHeadings splits before each recognised section heading while keeping
// the heading attached to the block that follows it.
func splitKeepHeadings(text string) []string {
	locs := sectionHead.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return []string{text}
	}
	var parts []string
	prev := 0
	for _, loc := range locs {
		// loc[0] points at the leading "\n"; keep the heading with the next part.
		if loc[0] > prev {
			parts = append(parts, text[prev:loc[0]])
		}
		prev = loc[0] + 1 // skip the newline
	}
	parts = append(parts, text[prev:])
	return parts
}

func lastIndexBefore(s, sep string, limit int) int {
	if limit > len(s) {
		limit = len(s)
	}
	return strings.LastIndex(s[:limit], sep)
}

func runTool(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	if _, err := exec.LookPath(name); err != nil {
		return "", fmt.Errorf("%s not found on PATH", name)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
