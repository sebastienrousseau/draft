// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine_test

import (
	"fmt"

	"github.com/sebastienrousseau/draft/internal/engine"
)

func ExampleProviderNames() {
	fmt.Println(engine.ProviderNames()[0])
	// Output: claude
}
