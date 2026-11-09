// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package c2pa

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// A C2PA data hash may exclude byte ranges of the asset, so a manifest
// embedded in a file need not hash itself. Nothing checks how much is
// excluded: Buchanan showed in October 2026 that a credential excluding the
// whole file signs the hash of nothing and still validates, after which the
// file can be anything. draft's credential is a sidecar, embedded in nothing,
// so its data hash has no reason to exclude a single byte, and one that does
// is treated as not covering the article.

// uncovered returns why the active manifest's hard binding does not cover the
// whole article, or "" when it does. store is the manifest's assertion store
// from c2patool --detailed; matched is whether c2patool reported the data
// hash matched the article's bytes.
func uncovered(store map[string]json.RawMessage, matched bool) string {
	hashes := 0
	// Sorted so a credential with several faults is always described the same way.
	for _, label := range slices.Sorted(maps.Keys(store)) {
		if !strings.HasPrefix(label, "c2pa.hash.") {
			continue
		}
		// c2pa-rs numbers a repeated label label__1, label__2, ...
		if base, _, _ := strings.Cut(label, "__"); base != "c2pa.hash.data" {
			return "the credential binds the article with " + label + ", not a data hash over its bytes"
		}
		var h struct {
			Exclusions []json.RawMessage `json:"exclusions"`
		}
		if err := json.Unmarshal(store[label], &h); err != nil {
			return "the credential's data hash cannot be read"
		}
		if n := len(h.Exclusions); n > 0 {
			return fmt.Sprintf("the credential's data hash excludes %d byte range(s) of the article, so the signature does not cover all of it", n)
		}
		hashes++
	}
	switch {
	case hashes == 0:
		return "the credential has no data hash binding it to the article"
	case !matched:
		return "c2patool did not confirm the article matches the credential's data hash"
	}
	return ""
}

// withReason formats a coverage reason for appending to an error, if any.
func withReason(reason string) string {
	if reason == "" {
		return ""
	}
	return "; " + reason
}
