// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package provenance

import (
	"bytes"
	"encoding/json"
)

// boundLabels are the assertions draft writes whose signed copy must match
// the manifest beside the article: the grounding, which carries the source
// and ledger digests, and the review and AI disclosure, which state who took
// responsibility for the text. c2patool stores each verbatim in the claim it
// signs; the actions and ingredients it rewrites are not compared.
var boundLabels = []string{GroundingLabel, ReviewLabel, AIDisclosureLabel}

// Unbound reports why a signed credential does not vouch for manifest, or ""
// when it does. signed maps each assertion label in the credential's claim to
// its data, as the verifier read it back.
//
// Without this a valid signature proves only that the article is unchanged:
// the digests of the sources and the ledger, and any review, live in the
// unsigned manifest and could be rewritten, or a review added, under a
// signature that still verifies. Both sides are decoded and re-encoded
// before comparing, so key order and spacing do not matter.
func Unbound(manifest []byte, signed map[string]json.RawMessage) string {
	var m struct {
		Assertions []struct {
			Label string          `json:"label"`
			Data  json.RawMessage `json:"data"`
		} `json:"assertions"`
	}
	if err := json.Unmarshal(manifest, &m); err != nil {
		return "the manifest cannot be read"
	}
	beside := map[string]json.RawMessage{}
	for _, a := range m.Assertions {
		beside[a.Label] = a.Data
	}
	for _, label := range boundLabels {
		mine, inManifest := beside[label]
		theirs, inCredential := signed[label]
		switch {
		case !inManifest && !inCredential:
		case !inCredential:
			return "its " + label + " assertion was not signed"
		case !inManifest:
			return "the signed " + label + " assertion is missing from the manifest"
		case !sameJSON(mine, theirs):
			return "its " + label + " assertion differs from the one the credential signed"
		}
	}
	return ""
}

// sameJSON reports whether a and b encode the same JSON value.
func sameJSON(a, b json.RawMessage) bool {
	ca, okA := canonicalJSON(a)
	cb, okB := canonicalJSON(b)
	return okA && okB && bytes.Equal(ca, cb)
}

// canonicalJSON re-encodes v with sorted keys. UseNumber keeps every figure
// as written rather than round-tripping it through float64.
func canonicalJSON(v json.RawMessage) ([]byte, bool) {
	dec := json.NewDecoder(bytes.NewReader(v))
	dec.UseNumber()
	var x any
	if err := dec.Decode(&x); err != nil {
		return nil, false
	}
	// A value just decoded from JSON always encodes.
	out, _ := json.Marshal(x)
	return out, true
}
