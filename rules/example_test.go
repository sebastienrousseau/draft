// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package rules_test

import (
	"fmt"

	"github.com/sebastienrousseau/draft/rules"
)

// WordForms expands a banned word into the inflections the validator must
// also catch, so "leverage" covers "leverages", "leveraging", and the rest.
func ExampleWordForms() {
	for _, f := range rules.WordForms("leverage") {
		fmt.Printf("%s (%s)\n", f.Form, f.Kind)
	}
	// Output:
	// leverage (base)
	// leverages (s)
	// leveraged (ed)
	// leveraging (ing)
	// leveragely (ly)
}
