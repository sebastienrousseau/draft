// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pdf

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

// SplitSections used to fall back to a fixed byte offset when it found neither
// a paragraph nor a sentence boundary, slicing multi-byte runes in half. The
// mangled bytes then went into a prompt and into the text a claim's quote is
// matched against.
func TestSplitSectionsNeverSplitsARune(t *testing.T) {
	// A leading ASCII byte shifts every following 3-byte rune so the fixed
	// MaxSectionChars cut cannot land on a boundary by luck. No "\n\n" and no
	// ". " anywhere, so the fallback is the path taken.
	text := "x" + strings.Repeat("測", 3000)

	secs := SplitSections("src", text)
	if len(secs) < 2 {
		t.Fatalf("expected the text to be split, got %d section(s)", len(secs))
	}
	var rebuilt strings.Builder
	for i, s := range secs {
		if !utf8.ValidString(s.Body) {
			t.Errorf("section %d is not valid UTF-8", i)
		}
		rebuilt.WriteString(s.Body)
	}
	// Splitting must lose nothing but the whitespace it trims on.
	if got, want := utf8.RuneCountInString(rebuilt.String()), utf8.RuneCountInString(text); got != want {
		t.Errorf("rebuilt %d runes, want %d", got, want)
	}
}

func TestSplitSectionsStillPrefersRealBoundaries(t *testing.T) {
	para := strings.Repeat("A sentence that says something. ", 200)
	secs := SplitSections("src", para+"\n\n"+para)
	for _, s := range secs {
		if len(s.Body) > MaxSectionChars {
			t.Errorf("section of %d bytes exceeds the cap", len(s.Body))
		}
	}
}

// A filename beginning with "-" would be parsed as a flag by pdftotext and
// textutil. Extract resolves to an absolute path so that cannot happen.
func TestExtractResolvesToAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	name := "-oh-dear.txt"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("safe content"), 0o644); err != nil {
		t.Fatal(err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	got, err := Extract(context.Background(), name)
	if err != nil {
		t.Fatalf("Extract(%q): %v", name, err)
	}
	if got != "safe content" {
		t.Errorf("Extract = %q", got)
	}
}

func TestReadCappedRejectsOversizeFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 32), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := readCapped(path); err != nil {
		t.Errorf("a small file must be readable, got %v", err)
	}
	if _, err := readCappedLimit(path, 8); err == nil {
		t.Error("expected an oversize file to be refused")
	}
	if _, err := readCapped(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestReadCappedPropagatesReadFailure(t *testing.T) {
	if _, err := readCappedLimit(t.TempDir(), MaxSourceBytes); err == nil {
		t.Fatal("expected reading a directory to fail")
	}
}

func TestLimitedWriterTruncatesWithoutFailing(t *testing.T) {
	var buf bytes.Buffer
	w := &limitedWriter{w: &buf, remaining: 5}

	n, err := w.Write([]byte("abc"))
	if n != 3 || err != nil {
		t.Fatalf("Write = (%d, %v)", n, err)
	}
	// Straddles the cap: reports the full length so the producer does not see
	// a short write, but only stores what fits.
	n, err = w.Write([]byte("defgh"))
	if n != 5 || err != nil {
		t.Fatalf("straddling Write = (%d, %v)", n, err)
	}
	// Past the cap: silently discarded.
	if n, err = w.Write([]byte("ijk")); n != 3 || err != nil {
		t.Fatalf("post-cap Write = (%d, %v)", n, err)
	}
	if buf.String() != "abcde" {
		t.Errorf("buffered %q, want %q", buf.String(), "abcde")
	}
}

func TestLimitedWriterPropagatesUnderlyingFailure(t *testing.T) {
	w := &limitedWriter{w: errorWriter{}, remaining: 1}
	if _, err := w.Write([]byte("too long")); err == nil {
		t.Fatal("expected the underlying writer error")
	}
}

// runTool must surface the tool's own diagnostic instead of "exit status 1".
func TestRunToolSurfacesStderr(t *testing.T) {
	_, err := runTool(context.Background(), 5*time.Second, "sh", "-c", "echo 'the real reason' >&2; exit 1")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "the real reason") {
		t.Errorf("error should carry the tool's stderr, got %q", err)
	}
}

func TestRunToolReportsTimeout(t *testing.T) {
	_, err := runTool(context.Background(), 10*time.Millisecond, "sh", "-c", "sleep 5")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected a timeout error, got %v", err)
	}
}

func TestRunToolReportsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runTool(ctx, time.Second, "sh", "-c", "true"); err == nil {
		t.Error("expected a cancellation error")
	}
}

func TestRunToolReturnsSilentExitError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	path := filepath.Join(t.TempDir(), "silent-fail")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := runTool(context.Background(), time.Second, path); err == nil {
		t.Fatal("expected the silent non-zero exit to be returned")
	}
}

func TestExtractRejectsDOCXOutsideMacOS(t *testing.T) {
	original := operatingSystem
	operatingSystem = "linux"
	t.Cleanup(func() { operatingSystem = original })

	path := filepath.Join(t.TempDir(), "paper.docx")
	if _, err := Extract(context.Background(), path); err == nil || !strings.Contains(err.Error(), "requires macOS") {
		t.Fatalf("expected the platform error, got %v", err)
	}
}

func TestNormaliseSpaceFoldsLineEndings(t *testing.T) {
	got := NormaliseSpace("a\r\nb\rc\n\n\n\n\nd")
	if want := "a\nb\nc\n\n\nd"; got != want {
		t.Errorf("NormaliseSpace = %q, want %q", got, want)
	}
	// The no-blank-run path must be identical.
	if got := NormaliseSpace("  a\r\nb  "); got != "a\nb" {
		t.Errorf("NormaliseSpace = %q", got)
	}
}
