// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pdf

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunToolSuccess(t *testing.T) {
	// Use `go` (guaranteed on PATH) so the test is portable across OSes —
	// `echo`/`false` are shell builtins that are not executables on Windows.
	out, err := runTool(context.Background(), 10*time.Second, "go", "version")
	if err != nil || !strings.Contains(out, "go") {
		t.Errorf("runTool go version failed: %q %v", out, err)
	}
}

func TestRunToolMissingBinary(t *testing.T) {
	if _, err := runTool(context.Background(), time.Second, "definitely-not-a-real-binary-xyz"); err == nil {
		t.Error("missing binary should error")
	}
}

func TestRunToolNonZeroExit(t *testing.T) {
	// `go help <unknown>` exits non-zero on every platform.
	if _, err := runTool(context.Background(), 10*time.Second, "go", "help", "zzznotacommand"); err == nil {
		t.Error("non-zero exit should error")
	}
}

func TestExtractPDFInvalid(t *testing.T) {
	// Not a real PDF -> pdftotext exits non-zero -> error from the .pdf branch.
	p := filepath.Join(t.TempDir(), "fake.pdf")
	os.WriteFile(p, []byte("not a pdf"), 0o644)
	if _, err := Extract(context.Background(), p); err == nil || !strings.Contains(err.Error(), "pdftotext") {
		t.Errorf("expected pdftotext error, got %v", err)
	}
}

func TestExtractDOCXInvalid(t *testing.T) {
	p := filepath.Join(t.TempDir(), "fake.docx")
	os.WriteFile(p, []byte("not a docx"), 0o644)
	// textutil either errors or returns empty; either way the .docx branch runs.
	_, _ = Extract(context.Background(), p)
}

func TestCauseLine(t *testing.T) {
	cases := map[string]string{
		"single line":           "docling: conversion failed",
		"warning before error":  "Syntax Warning: odd\nSyntax Error: Couldn't open file 'x.pdf'",
		"no error named":        "first line\nlast line",
		"exception then a note": "Traceback:\n  File \"a.py\"\nValueError: bad page\nWARNING: failed to convert",
	}
	want := map[string]string{
		"single line":           "docling: conversion failed",
		"warning before error":  "Syntax Error: Couldn't open file 'x.pdf'",
		"no error named":        "last line",
		"exception then a note": "ValueError: bad page",
	}
	for name, in := range cases {
		if got := causeLine(in); got != want[name] {
			t.Errorf("%s: causeLine = %q, want %q", name, got, want[name])
		}
	}
}

func TestTailWriterKeepsTheEnd(t *testing.T) {
	w := &tailWriter{max: 5}
	for _, s := range []string{"abc", "defg", "hi"} {
		if n, err := w.Write([]byte(s)); n != len(s) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", s, n, err)
		}
	}
	if got := string(w.buf); got != "efghi" {
		t.Errorf("kept %q, want the last five bytes %q", got, "efghi")
	}
}
