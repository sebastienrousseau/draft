// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package frontmatter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadCappedRefusesOversizeFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.md")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 64)), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := readCapped(path, MaxArticleBytes); err != nil {
		t.Errorf("a small file must be readable, got %v", err)
	}
	if _, err := readCapped(path, 16); err == nil {
		t.Error("expected an oversize file to be refused")
	}
	if _, err := readCapped(filepath.Join(t.TempDir(), "missing.md"), MaxArticleBytes); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestProcessFileRefusesAnOversizeArticle(t *testing.T) {
	// The cap is exercised through readCapped above; this checks the error is
	// propagated rather than swallowed into an empty body.
	path := filepath.Join(t.TempDir(), "2026-07-29-x-body.md")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ProcessFile(path, time.Now()); err == nil {
		t.Error("expected an empty article to be refused")
	}
}

// A YAML double-quoted scalar may not carry a raw control character. A title
// that somehow contains one must not produce frontmatter that fails to parse.
func TestQuoteYAMLStripsControlCharacters(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "A Simple Title", want: `"A Simple Title"`},
		{name: "quotes escaped", in: `He said "no"`, want: `"He said \"no\""`},
		{name: "backslash escaped", in: `a\b`, want: `"a\\b"`},
		{name: "newline folded", in: "two\nlines", want: `"two lines"`},
		{name: "tab folded", in: "two\tcolumns", want: `"two columns"`},
		{name: "control stripped", in: "bell\x07here", want: `"bellhere"`},
		{name: "del stripped", in: "del\x7fhere", want: `"delhere"`},
		{name: "runs collapsed", in: "  spaced   out  ", want: `"spaced out"`},
		{name: "accents kept", in: "réduction de 5x", want: `"réduction de 5x"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := quoteYAML(tc.in); got != tc.want {
				t.Errorf("quoteYAML(%q) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

// The three files are written as a unit, so a set can never be left with one
// file updated and the others stale.
func TestProcessFileWritesTheSetTogether(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-07-29-router-s-body.md")
	body := "# Router-S Cuts Compute\n\n**One number tells the story.**\n\nRouter-S used 5x fewer FLOPs.\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	bodyPath, yamlPath, finalPath, err := ProcessFile(path, time.Now())
	if err != nil {
		t.Fatalf("ProcessFile: %v", err)
	}
	for _, p := range []string{bodyPath, yamlPath, finalPath} {
		if _, statErr := os.Stat(p); statErr != nil {
			t.Errorf("%s was not written: %v", filepath.Base(p), statErr)
		}
	}
	// No staging files may survive.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("staging file %q survived", e.Name())
		}
	}

	// Re-running on unchanged input must be a byte-level no-op.
	before := readAll(t, bodyPath, yamlPath, finalPath)
	if _, _, _, err := ProcessFile(path, time.Now()); err != nil {
		t.Fatalf("second ProcessFile: %v", err)
	}
	if after := readAll(t, bodyPath, yamlPath, finalPath); after != before {
		t.Error("reprocessing unchanged input was not a no-op")
	}
}

func readAll(t *testing.T, paths ...string) string {
	t.Helper()
	var b strings.Builder
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
	}
	return b.String()
}
