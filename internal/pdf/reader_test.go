// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pdf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeDocling puts a stand-in `docling` first on PATH. It ignores its input
// and writes the given Markdown to <output>/<input stem>.md, the way the real
// CLI names its output; an empty body writes an empty file, and "fail" exits
// non-zero. "noisy-fail" fails the way the real CLI does when a model cannot
// be fetched: a harmless warning first, a long traceback, the cause last.
// "scan" stands in for a PDF with no text layer: only an image placeholder
// without OCR, and text with it. Each call's arguments are appended, one line
// per call, to the file the function returns.
func fakeDocling(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake reader is a shell script")
	}
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls.log")
	script := `#!/bin/sh
echo "$*" >> '` + calls + `'
out=""
ocr=1
while [ $# -gt 1 ]; do
  if [ "$1" = "--output" ]; then out="$2"; shift; fi
  if [ "$1" = "--no-ocr" ]; then ocr=0; fi
  shift
done
in="$1"
stem=$(basename "$in"); stem="${stem%.*}"
body='` + strings.ReplaceAll(body, "'", `'\''`) + `'
if [ "$body" = "fail" ]; then echo "docling: conversion failed" >&2; exit 1; fi
if [ "$body" = "noisy-fail" ]; then
  echo "W1120 redirects.py:35] NOTE: Redirects are currently not supported in MacOs." >&2
  i=0; while [ $i -lt 400 ]; do echo "  File \"frame$i.py\", line $i, in run" >&2; i=$((i+1)); done
  echo "rapidocr.utils.download_file.DownloadFileException: Failed to download https://example.invalid/det.pth" >&2
  echo "WARNING docling.cli.main: Document $in failed to convert." >&2
  exit 1
fi
if [ "$body" = "scan" ]; then
  if [ $ocr = 1 ]; then body='OCR text with a value of 5.'; else body='<!-- image -->'; fi
fi
if [ "$body" = "nothing" ]; then exit 0; fi
if [ "$body" = "renamed" ]; then printf 'Renamed output with a value of 7.' > "$out/converted-document.md"; exit 0; fi
printf '%s' "$body" > "$out/$stem.md"
`
	if err := os.WriteFile(filepath.Join(dir, "docling"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return calls
}

func TestValidateReader(t *testing.T) {
	for _, ok := range []string{"", ReaderPDFToText, ReaderDocling} {
		if err := ValidateReader(ok); err != nil {
			t.Errorf("ValidateReader(%q) = %v", ok, err)
		}
	}
	err := ValidateReader("marker")
	if err == nil || !strings.Contains(err.Error(), `"marker"`) || !strings.Contains(err.Error(), "docling") {
		t.Errorf("an unknown reader should be named and the choices listed, got %v", err)
	}
	if got := Readers(); got[0] != ReaderPDFToText || len(got) != 2 {
		t.Errorf("Readers() = %v", got)
	}
	if _, err := ExtractWith(context.Background(), "x.pdf", "marker"); err == nil {
		t.Error("ExtractWith must refuse an unknown reader before touching the file")
	}
}

func TestDoclingReaderProducesNormalisedText(t *testing.T) {
	fakeDocling(t, "<!-- image -->\n\n## Results\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n<!-- image -->\n\nThe score was 0.82.\n")
	p := filepath.Join(t.TempDir(), "paper.pdf")
	if err := os.WriteFile(p, []byte("%PDF-1.4 not really"), 0o644); err != nil {
		t.Fatal(err)
	}
	text, err := ExtractWith(context.Background(), p, ReaderDocling)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "<!-- image -->") {
		t.Errorf("image placeholders should be stripped:\n%s", text)
	}
	if !strings.Contains(text, "| 1 | 2 |") || !strings.Contains(text, "The score was 0.82.") {
		t.Errorf("table and prose should survive:\n%s", text)
	}
	if strings.HasPrefix(text, "\n") {
		t.Errorf("leading blank lines should be normalised away: %q", text[:20])
	}
}

// Docling reads DOCX itself, so with it chosen the macOS-only textutil path
// is never needed.
func TestDoclingReaderHandlesDOCXOffMacOS(t *testing.T) {
	fakeDocling(t, "A DOCX paragraph with a result of 42.")
	original := operatingSystem
	operatingSystem = "linux"
	t.Cleanup(func() { operatingSystem = original })
	p := filepath.Join(t.TempDir(), "memo.docx")
	if err := os.WriteFile(p, []byte("PK not really"), 0o644); err != nil {
		t.Fatal(err)
	}
	text, err := ExtractWith(context.Background(), p, ReaderDocling)
	if err != nil || !strings.Contains(text, "result of 42") {
		t.Errorf("got (%q, %v)", text, err)
	}
	// The default reader still refuses DOCX off macOS.
	if _, err := ExtractWith(context.Background(), p, ReaderPDFToText); err == nil || !strings.Contains(err.Error(), "macOS") {
		t.Errorf("pdftotext reader should still need textutil, got %v", err)
	}
}

func TestDoclingReaderFailures(t *testing.T) {
	p := filepath.Join(t.TempDir(), "paper.pdf")
	if err := os.WriteFile(p, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Run("not installed", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		_, err := ExtractWith(context.Background(), p, ReaderDocling)
		if err == nil || !strings.Contains(err.Error(), "docling not found") || !strings.Contains(err.Error(), "--reader pdftotext") {
			t.Errorf("a missing docling should say how to proceed, got %v", err)
		}
	})
	t.Run("conversion fails", func(t *testing.T) {
		fakeDocling(t, "fail")
		_, err := ExtractWith(context.Background(), p, ReaderDocling)
		if err == nil || !strings.Contains(err.Error(), "conversion failed") {
			t.Errorf("the tool's stderr should be surfaced, got %v", err)
		}
	})
	t.Run("no output", func(t *testing.T) {
		fakeDocling(t, "nothing")
		_, err := ExtractWith(context.Background(), p, ReaderDocling)
		if err == nil || !strings.Contains(err.Error(), "produced no Markdown") {
			t.Errorf("got %v", err)
		}
	})
	t.Run("output named differently", func(t *testing.T) {
		// Docling derives the name from the input; if a future version
		// chooses otherwise, whatever Markdown it wrote is still taken.
		fakeDocling(t, "renamed")
		text, err := ExtractWith(context.Background(), p, ReaderDocling)
		if err != nil || !strings.Contains(text, "value of 7") {
			t.Errorf("got (%q, %v)", text, err)
		}
	})
	t.Run("empty output", func(t *testing.T) {
		fakeDocling(t, "<!-- image -->\n")
		_, err := ExtractWith(context.Background(), p, ReaderDocling)
		if !errors.Is(err, ErrNoTextLayer) {
			t.Errorf("an image-only document should be reported as having no text, got %v", err)
		}
	})
	t.Run("cause buried under warnings", func(t *testing.T) {
		// The real CLI opens with a platform warning and ends a long
		// traceback with the exception; reporting the first line told the
		// user about the warning and hid the failed download.
		fakeDocling(t, "noisy-fail")
		_, err := ExtractWith(context.Background(), p, ReaderDocling)
		if err == nil || !strings.Contains(err.Error(), "Failed to download") {
			t.Errorf("the exception should be reported, got %v", err)
		}
		if err != nil && strings.Contains(err.Error(), "Redirects") {
			t.Errorf("the leading warning should not stand in for the cause, got %v", err)
		}
	})
}

// A PDF with a text layer needs no OCR, and OCR fetches models over the
// network on first use, so a text PDF must convert with OCR off and in a
// single pass.
func TestDoclingSkipsOCRWhenTextIsPresent(t *testing.T) {
	calls := fakeDocling(t, "A text layer with a value of 3.")
	p := filepath.Join(t.TempDir(), "paper.pdf")
	if err := os.WriteFile(p, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	text, err := ExtractWith(context.Background(), p, ReaderDocling)
	if err != nil || !strings.Contains(text, "value of 3") {
		t.Fatalf("got (%q, %v)", text, err)
	}
	log := doclingCalls(t, calls)
	if len(log) != 1 || !strings.Contains(log[0], "--no-ocr") {
		t.Errorf("want one call with --no-ocr, got %q", log)
	}
}

// A scan has nothing to read without OCR, so the reader falls back to it
// rather than reporting a text PDF's failure mode.
func TestDoclingFallsBackToOCRForAScan(t *testing.T) {
	calls := fakeDocling(t, "scan")
	p := filepath.Join(t.TempDir(), "scan.pdf")
	if err := os.WriteFile(p, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	text, err := ExtractWith(context.Background(), p, ReaderDocling)
	if err != nil || !strings.Contains(text, "OCR text with a value of 5.") {
		t.Fatalf("got (%q, %v)", text, err)
	}
	log := doclingCalls(t, calls)
	if len(log) != 2 || !strings.Contains(log[0], "--no-ocr") || strings.Contains(log[1], "--no-ocr") {
		t.Errorf("want a pass without OCR, then one with it, got %q", log)
	}
}

func doclingCalls(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

// Markdown and text sources never go through a reader: choosing docling for
// a queue that mixes PDFs and notes must not shell out for the notes.
func TestReaderIgnoredForPlainSources(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	p := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(p, []byte("# Notes\n\nA fact."), 0o644); err != nil {
		t.Fatal(err)
	}
	text, err := ExtractWith(context.Background(), p, ReaderDocling)
	if err != nil || !strings.Contains(text, "A fact.") {
		t.Errorf("got (%q, %v)", text, err)
	}
}
