package validate

import (
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
