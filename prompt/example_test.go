// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package prompt_test

import (
	"fmt"
	"strings"

	"github.com/sebastienrousseau/draft/prompt"
	"github.com/sebastienrousseau/draft/rules"
)

// Writing builds the grounded brief: the verified claim ledger is the only
// factual substrate the model is given.
func ExampleWriting() {
	ledger := "1. Router-S used 5x fewer FLOPs [demonstrated]"
	p := prompt.Writing("", ledger, rules.MinWords, rules.MaxWords)
	fmt.Println(strings.Contains(p, ledger))
	// Output: true
}

// Claim asks a backend to mine verifiable facts out of one source section.
func ExampleClaim() {
	p := prompt.Claim("Router-S used 5x fewer FLOPs than the dense baseline.")
	fmt.Println(strings.Contains(p, "SOURCE_QUOTE"))
	// Output: true
}
