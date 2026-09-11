// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package gbnf

import (
	"testing"

	"github.com/sebastienrousseau/draft/claims"
)

func BenchmarkFromJSONSchema(b *testing.B) {
	schema := claims.Schema()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := FromJSONSchema(schema); err != nil {
			b.Fatal(err)
		}
	}
}
