// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"fmt"
	"io"

	"github.com/sebastienrousseau/draft/config"
)

// runTrustConfig records ./draft.toml as trusted. Until then draft ignores
// it, because it can choose the engine, the output folder and the signing key
// for whoever runs draft in that folder.
func runTrustConfig(stdout, stderr io.Writer) int {
	path, err := config.TrustProjectConfig()
	if err != nil {
		fmt.Fprintln(stderr, "draft: trust config:", err)
		return 1
	}
	fmt.Fprintf(stdout, "draft: trusted %s as it now stands; editing it lapses the trust\n", path)
	return 0
}
