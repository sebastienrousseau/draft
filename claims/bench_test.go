// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import "testing"

var benchText = `CLAIM: The system reached a score of 0.82
SOURCE_QUOTE: "reached a score of 0.82 on the test set"
TYPE: metric
STRENGTH: demonstrated
---`

func BenchmarkParse(b *testing.B) {
	src := "reached a score of 0.82 on the test set"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Parse(benchText, src)
	}
}
