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
