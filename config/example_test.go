// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package config_test

import (
	"fmt"

	"github.com/sebastienrousseau/draft/config"
)

// Flags win over environment variables, which win over defaults.
func ExampleLoad() {
	cfg := config.Load(config.Flags{Engine: "ollama", ContextLength: 2048})
	fmt.Println(cfg.Engine, cfg.ContextLength)
	// Output: ollama 2048
}
