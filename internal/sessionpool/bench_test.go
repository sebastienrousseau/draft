// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package sessionpool

import (
	"context"
	"testing"
	"time"
)

func BenchmarkGetRelease(b *testing.B) {
	p := New(time.Hour)
	ctx := context.Background()
	start := func(context.Context) (Session, error) { return newFake(), nil }
	// Prime the key so the loop measures the steady-state reuse path, not the
	// one-off start.
	if _, err := p.Get(ctx, "k", start); err != nil {
		b.Fatal(err)
	}
	p.Release("k")

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := p.Get(ctx, "k", start); err != nil {
			b.Fatal(err)
		}
		p.Release("k")
	}
}
