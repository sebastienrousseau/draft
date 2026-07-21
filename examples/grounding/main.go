// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Command grounding demonstrates draft's offline building blocks with no LLM
// or network: it verifies claims against a source, renders the ledger, builds a
// writing prompt, and validates a draft against the house rules.
//
// Run it with:
//
//	go run ./examples/grounding
package main

import (
	"fmt"

	"github.com/sebastienrousseau/draft/internal/claims"
	"github.com/sebastienrousseau/draft/internal/prompt"
	"github.com/sebastienrousseau/draft/internal/rules"
	"github.com/sebastienrousseau/draft/internal/validate"
)

func main() {
	source := "Router-S reached a validation loss of 3.41 and used 5x fewer FLOPs than the dense baseline."

	// A model's extraction output. The second record's quote is not in the
	// source, so it is dropped during verification.
	raw := `CLAIM: Router-S reached a validation loss of 3.41
SOURCE_QUOTE: "reached a validation loss of 3.41"
TYPE: metric
STRENGTH: demonstrated
---
CLAIM: Router-S triples inference speed
SOURCE_QUOTE: "triples inference speed on every device"
TYPE: result
STRENGTH: demonstrated
---`

	records, dropped := claims.Parse(raw, source)
	fmt.Printf("verified %d claim(s), dropped %d\n\n", len(records), dropped)
	fmt.Println(claims.RenderLedger(records, dropped))

	ledger := claims.RenderPromptLedger(records, 45, 14000)
	writingPrompt := prompt.Writing("", ledger, rules.MinWords, rules.MaxWords)
	fmt.Printf("built a %d-character grounded writing prompt\n\n", len(writingPrompt))

	// A structurally incomplete draft: the validator explains what is missing.
	draft := "# Router-S\n\nSome prose about the result that is far too short."
	fmt.Println("validation errors for an incomplete draft:")
	for _, e := range validate.Errors(draft) {
		fmt.Println("  -", e)
	}
}
