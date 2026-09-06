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
// non-zero.
func fakeDocling(t *testing.T, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake reader is a shell script")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
out=""
while [ $# -gt 1 ]; do
  if [ "$1" = "--output" ]; then out="$2"; shift; fi
  shift
done
in="$1"
stem=$(basename "$in"); stem="${stem%.*}"
body='` + strings.ReplaceAll(body, "'", `'\''`) + `'
if [ "$body" = "fail" ]; then echo "docling: conversion failed" >&2; exit 1; fi
if [ "$body" = "nothing" ]; then exit 0; fi
if [ "$body" = "renamed" ]; then printf 'Renamed output with a value of 7.' > "$out/converted-document.md"; exit 0; fi
printf '%s' "$body" > "$out/$stem.md"
`
	if err := os.WriteFile(filepath.Join(dir, "docling"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
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
