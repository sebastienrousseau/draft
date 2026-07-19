package prompt

import "testing"

func BenchmarkWriting(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Writing("", "some ledger content")
	}
}
