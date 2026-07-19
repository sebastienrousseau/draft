// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Command providers prints draft's supported token-free session providers, in
// auto-selection preference order, and whether each CLI is installed on PATH.
//
// Run it with:
//
//	go run ./examples/providers
package main

import (
	"fmt"
	"os/exec"

	"github.com/sebastienrousseau/draft/internal/engine"
)

func main() {
	fmt.Printf("%-14s  %-9s  %-12s  %s\n", "PROVIDER", "INSTALLED", "STATUS", "DEFAULT MODEL")
	for _, name := range engine.ProviderNames() {
		p, _ := engine.LookupProvider(name)
		installed := "no"
		if _, err := exec.LookPath(p.Bin); err == nil {
			installed = "yes"
		}
		status := "stable"
		if p.Experimental {
			status = "experimental"
		}
		model := p.DefaultModel
		if model == "" {
			model = "(session default)"
		}
		fmt.Printf("%-14s  %-9s  %-12s  %s\n", p.Name, installed, status, model)
	}
	fmt.Println("\nauto mode uses stable providers; add --experimental to include the rest.")
}
