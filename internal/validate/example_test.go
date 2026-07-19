// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate_test

import (
	"fmt"

	"github.com/sebastienrousseau/draft/internal/validate"
)

func ExampleErrors() {
	fmt.Println(validate.Errors("not a valid article")[0])
	// Output: body-only mode must start with a Markdown H1
}
