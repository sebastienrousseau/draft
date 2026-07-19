package engine_test

import (
	"fmt"

	"github.com/sebastienrousseau/draft/internal/engine"
)

func ExampleProviderNames() {
	fmt.Println(engine.ProviderNames()[0])
	// Output: claude
}
