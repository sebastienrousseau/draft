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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxSectionChars bounds a single extraction section so a small local model is
// never asked to hold more than it can attend to at once.
const MaxSectionChars = 4500

var (
	multiBlank  = regexp.MustCompile(`\n{4,}`)
	stopSection = regexp.MustCompile(`(?im)^\s*(?:#{1,6}\s+)?(?:references|bibliography|acknowledge?ments|appendix)\b`)
	sectionHead = regexp.MustCompile(`\n(?:(?:Abstract|Introduction|Related Work|Method|Methods|Experiments|Results|Discussion|Limitations|Conclusion|References)\b)`)
)

// MaxSourceBytes caps how much of a single source file is read. Sources are
// research papers; anything past this is a mistake or an attack, and reading it
// would cost several times its size in peak memory once normalisation runs.
const MaxSourceBytes = 256 << 20

// Extract returns the normalised plain text of a .pdf, .docx, .md, or .txt
// file. Unknown suffixes yield an error so the caller can skip them cleanly.
func Extract(ctx context.Context, path string) (string, error) {
	// Resolve to an absolute path before any of it reaches a subprocess.
	// pdftotext and textutil both parse leading-dash arguments as flags, so a
	// file called "-v.pdf" would otherwise be argument injection rather than
	// an input. filepath.Abs guarantees a leading separator.
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", path, err)
	}
	path = abs

	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".txt":
		b, err := readCapped(path)
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
				// No paragraph or sentence boundary to cut on, so fall back to
				// the byte limit — backed up to a rune boundary. Slicing a
				// multi-byte rune in half produces invalid UTF-8 in both the
				// prompt and the text a claim's quote is later matched against.
				cut = runeBoundaryAtOrBefore(part, MaxSectionChars)
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

// lineEndings folds CRLF and lone CR to LF in a single pass.
var lineEndings = strings.NewReplacer("\r\n", "\n", "\r", "\n")

// NormaliseSpace canonicalises line endings and collapses runs of blank lines.
func NormaliseSpace(text string) string {
	// One Replacer pass rather than two ReplaceAll passes: each pass copies the
	// whole document, and these run on every source file.
	text = lineEndings.Replace(text)
	if multiBlank.MatchString(text) {
		text = multiBlank.ReplaceAllString(text, "\n\n\n")
	}
	return strings.TrimSpace(text)
}

// runeBoundaryAtOrBefore returns the largest index <= limit that starts a UTF-8
// rune, so slicing s[:index] can never split one in half. It walks back at most
// three bytes — the longest a UTF-8 continuation run can be.
func runeBoundaryAtOrBefore(s string, limit int) int {
	if limit >= len(s) {
		return len(s)
	}
	for i := 0; i < utf8.UTFMax && limit-i > 0; i++ {
		if utf8.RuneStart(s[limit-i]) {
			return limit - i
		}
	}
	return limit
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

// readCapped reads a file, refusing anything larger than MaxSourceBytes rather
// than pulling it all into memory first.
func readCapped(path string) ([]byte, error) {
	return readCappedLimit(path, MaxSourceBytes)
}

// readCappedLimit is readCapped with an injectable limit, so the refusal path
// can be tested without writing a quarter of a gigabyte to disk.
func readCappedLimit(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	if info, statErr := f.Stat(); statErr == nil && info.Size() > limit {
		return nil, fmt.Errorf("%s is %d bytes; the limit is %d", filepath.Base(path), info.Size(), limit)
	}
	// LimitReader guards the case where Stat lied or the file grew after it.
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s exceeds the %d byte limit", filepath.Base(path), limit)
	}
	return b, nil
}

// runTool executes an extraction helper with its output bounded and its stderr
// preserved. Using exec.Output and returning the bare error yields
// "exit status 1" and discards the tool's own diagnostic, which is the message
// the user actually needs.
func runTool(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	if _, err := exec.LookPath(name); err != nil {
		return "", fmt.Errorf("%s not found on PATH", name)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, remaining: MaxSourceBytes}
	cmd.Stderr = &limitedWriter{w: &stderr, remaining: 8 << 10}

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); errors.Is(ctxErr, context.DeadlineExceeded) {
			return "", fmt.Errorf("%s timed out after %s", name, timeout)
		} else if ctxErr != nil {
			return "", ctxErr
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, firstLine(msg))
		}
		return "", err
	}
	return stdout.String(), nil
}

// limitedWriter caps how much a subprocess can hand back, so a runaway tool
// cannot exhaust memory. Writes past the cap are discarded, not an error: a
// truncated extraction is still worth the sections it produced.
type limitedWriter struct {
	w         io.Writer
	remaining int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.remaining <= 0 {
		return len(p), nil
	}
	if len(p) > l.remaining {
		if _, err := l.w.Write(p[:l.remaining]); err != nil {
			return 0, err
		}
		l.remaining = 0
		return len(p), nil
	}
	l.remaining -= len(p)
	return l.w.Write(p)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
