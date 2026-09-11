// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package gbnf_test

import (
	"fmt"

	"github.com/sebastienrousseau/draft/internal/gbnf"
)

func ExampleFromJSONSchema() {
	schema := map[string]any{"type": "string", "enum": []any{"yes", "no"}}
	g, err := gbnf.FromJSONSchema(schema)
	if err != nil {
		panic(err)
	}
	fmt.Print(g)
	// Output:
	// root ::= enum0
	// enum0 ::= "\"yes\"" | "\"no\""
}
