// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims_test

import (
	"fmt"

	"github.com/sebastienrousseau/draft/claims"
)

func ExampleParse() {
	source := "The method used 5x fewer tokens than the baseline."
	raw := "CLAIM: used 5x fewer tokens\nSOURCE_QUOTE: \"used 5x fewer tokens\"\nTYPE: result\nSTRENGTH: demonstrated\n---"
	records, dropped := claims.Parse(raw, source)
	fmt.Printf("%d verified, %d dropped\n", len(records), dropped)
	// Output: 1 verified, 0 dropped
}

func ExampleSchema() {
	s := claims.Schema()
	items := s["items"].(map[string]any)
	props := items["properties"].(map[string]any)
	types := props["type"].(map[string]any)["enum"].([]any)
	fmt.Printf("%s of claims; type is one of %d values\n", s["type"], len(types))
	// Output: array of claims; type is one of 6 values
}
