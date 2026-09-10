// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pdf

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripTeXComments(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"whole-line comment removed", "keep\n% gone\nkeep2\n", "keep\n\nkeep2\n"},
		{"trailing comment removed", "text here % a note\n", "text here\n"},
		{"escaped percent kept", `100\% of the mass` + "\n", `100\% of the mass` + "\n"},
		{"double backslash then percent is a comment", `line\\% gone` + "\n", `line\\` + "\n"},
		{"no trailing newline", "abc % note", "abc"},
	} {
		if got := stripTeXComments(tc.in); got != tc.want {
			t.Errorf("%s: stripTeXComments(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestDeTeXDropsPreambleKeepsBody(t *testing.T) {
	src := `\documentclass{article}
\usepackage{amsmath} % noise
\begin{document}
The loss reached $\mathcal{L} = 0.82$ on the held-out set.
\end{document}
`
	got := deTeX(src)
	if strings.Contains(got, "documentclass") || strings.Contains(got, "usepackage") {
		t.Errorf("preamble survived: %q", got)
	}
	// The maths is exact text and must be preserved verbatim.
	if !strings.Contains(got, `$\mathcal{L} = 0.82$`) {
		t.Errorf("maths not preserved verbatim: %q", got)
	}
	if strings.Contains(got, "noise") {
		t.Errorf("comment survived: %q", got)
	}
}

func TestDeTeXWithoutDocumentEnvKeepsAll(t *testing.T) {
	// A fragment with no \begin{document} is kept whole (minus comments).
	src := "A bare fragment with $x^2$ and no document environment.\n% comment\n"
	got := deTeX(src)
	if !strings.Contains(got, "$x^2$") || strings.Contains(got, "comment") {
		t.Errorf("fragment handling wrong: %q", got)
	}
}

func TestExtractTeXEndToEnd(t *testing.T) {
	src := `\documentclass{article}
\begin{document}
% this line is a comment and must not appear
The system reached $99$ pages per second, a 3\% improvement.
\end{document}
`
	p := filepath.Join(t.TempDir(), "paper.tex")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Extract(context.Background(), p)
	if err != nil {
		t.Fatalf("Extract(.tex): %v", err)
	}
	if strings.Contains(got, "comment") || strings.Contains(got, "documentclass") {
		t.Errorf("noise leaked into extraction: %q", got)
	}
	// The escaped percent is a literal and the number is exact — both must
	// survive so a claim can quote them verbatim.
	for _, want := range []string{"$99$", `3\% improvement`} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in extraction, got %q", want, got)
		}
	}
}

func TestExtractTeXEmptyBodyIsNoTextLayer(t *testing.T) {
	// A .tex whose body is only comments has nothing to ground a claim in.
	src := "\\begin{document}\n% only a comment\n\\end{document}\n"
	p := filepath.Join(t.TempDir(), "empty.tex")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Extract(context.Background(), p); err != ErrNoTextLayer {
		t.Errorf("empty .tex body: err = %v, want ErrNoTextLayer", err)
	}
}
