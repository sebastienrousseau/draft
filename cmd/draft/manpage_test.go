// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func renderMan(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	if err := writeManPage(&b, "1.2.3", time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestManPageStructure(t *testing.T) {
	out := renderMan(t)
	if !strings.HasPrefix(out, `.TH DRAFT 1 "2026-09-02" "draft 1.2.3" "User Commands"`) {
		t.Errorf("bad title line: %q", strings.SplitN(out, "\n", 2)[0])
	}
	for _, want := range []string{
		".SH NAME", ".SH SYNOPSIS", ".SH DESCRIPTION", ".SH OPTIONS",
		".SH ENGINE", ".SH EXAMPLES", ".SH ENVIRONMENT", ".SH FILES",
		".SH EXIT STATUS", ".SH REQUIREMENTS", ".SH SEE ALSO",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("manpage missing section %s", want)
		}
	}
}

// A committed .1 is a copy of the CLI that nothing keeps honest. Generating it
// from the same tables --help renders is only useful if it really does cover
// all of them.
func TestManPageDocumentsEveryFlagAndExample(t *testing.T) {
	out := renderMan(t)
	for _, f := range flagHelp {
		name, _, _ := strings.Cut(f[0], " ")
		if name == "-h," {
			continue // rendered as part of the combined help entry
		}
		if !strings.Contains(out, roff(f[0])) {
			t.Errorf("manpage does not document %s", name)
		}
	}
	for _, ex := range usageExamples {
		if !strings.Contains(out, roff(ex[0])) {
			t.Errorf("manpage is missing example %q", ex[0])
		}
	}
	// Provider names come from the registry, not a second list.
	if !strings.Contains(out, roff("claude")) {
		t.Error("manpage does not list the providers")
	}
}

// A hyphen must reach the renderer escaped, or it is treated as a soft break;
// a leading dot would be read as a roff request.
func TestRoffEscaping(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"--engine", `\-\-engine`},
		{`a\b`, `a\eb`},
		{".leading", `\&.leading`},
		{"'quote", `\&'quote`},
		{"plain", "plain"},
	} {
		if got := roff(tc.in); got != tc.want {
			t.Errorf("roff(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCapitalise(t *testing.T) {
	if got := capitalise("one paper"); got != "One paper" {
		t.Errorf("capitalise() = %q", got)
	}
	if got := capitalise(""); got != "" {
		t.Errorf("capitalise(\"\") = %q", got)
	}
}

func TestRunManFlag(t *testing.T) {
	var out, errb strings.Builder
	if code := run([]string{"--man"}, &out, &errb); code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, errb.String())
	}
	if !strings.HasPrefix(out.String(), ".TH DRAFT 1 ") {
		t.Errorf("--man did not emit a manpage: %.60q", out.String())
	}
}

// failWriter reports an error on every write.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("no space left on device") }

func TestRunManReportsAWriteFailure(t *testing.T) {
	var errb strings.Builder
	if code := run([]string{"--man"}, failWriter{}, &errb); code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "draft:") {
		t.Errorf("stderr = %q", errb.String())
	}
}
