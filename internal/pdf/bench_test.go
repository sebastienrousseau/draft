// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pdf

import (
	"strings"
	"testing"
)

func BenchmarkSplitSections(b *testing.B) {
	text := "Abstract\n" + strings.Repeat("word ", 4000) + "\nResults\n" + strings.Repeat("more ", 2000)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = SplitSections("paper.pdf", text)
	}
}

func BenchmarkDeTeX(b *testing.B) {
	src := "\\documentclass{article}\n\\usepackage{amsmath} % noise\n\\begin{document}\n" +
		strings.Repeat("The loss reached $\\mathcal{L} = 0.82$. % a comment\nSome prose about the result.\n", 200) +
		"\\end{document}\n"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = deTeX(src)
	}
}
