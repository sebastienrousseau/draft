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
	fmt.Printf("%-14s  %-14s  %-9s  %s\n", "PROVIDER", "BINARY", "INSTALLED", "DEFAULT MODEL")
	for _, name := range engine.ProviderNames() {
		p, _ := engine.LookupProvider(name)
		installed := "no"
		if _, err := exec.LookPath(p.Bin); err == nil {
			installed = "yes"
		}
		model := p.DefaultModel
		if model == "" {
			model = "(session default)"
		}
		fmt.Printf("%-14s  %-14s  %-9s  %s\n", p.Name, p.Bin, installed, model)
	}
}
