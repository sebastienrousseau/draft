// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkErrors(b *testing.B) {
	md := "# Title\n\n<aside class=\"post-lead\"></aside>\n\nExecutive Summary\n\n## S\n\n" + strings.Repeat("word ", 800) + "."
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Errors(md)
	}
}

func BenchmarkFaithfulness(b *testing.B) {
	md := strings.Repeat("The result stands plainly. ", 200) + "."
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Faithfulness(md, nil)
	}
}

// BenchmarkDuplicateParagraphs guards the near-duplicate detector at a
// realistic article shape (many distinct paragraphs). The regression job
// compares it against the base revision, so a future change that reintroduces
// an O(n^2) blow-up in shingling or the pairwise scan shows up as a slowdown
// rather than sneaking in.
func BenchmarkDuplicateParagraphs(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&sb, "## Section %d\n\n", i)
		for w := 0; w < 60; w++ {
			fmt.Fprintf(&sb, "topic%d-word%d ", i, w)
		}
		sb.WriteString("\n\n")
	}
	article := sb.String()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = DuplicateParagraphIndexes(article)
	}
}
