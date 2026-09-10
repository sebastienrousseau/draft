// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

// ClaimTypes are the allowed values of a claim's Type field, in the order the
// extraction prompt presents them. They are the single source of truth the
// JSON Schema and the extraction prompt both mirror; a drift-guard test in the
// prompt package asserts the prompt still lists exactly these.
var ClaimTypes = []string{
	"metric", "mechanism", "definition", "method", "result", "limitation",
}

// ClaimStrengths are the allowed values of a claim's Strength field, in prompt
// order. Preserving hedging is the point of the field: a demonstrated result
// and a speculation-or-future-work note must not be flattened together.
var ClaimStrengths = []string{
	"demonstrated", "hedged", "speculation-or-future-work",
}

// Schema returns the JSON Schema (draft 2020-12) for a claim ledger: an array
// of claim objects, each with a non-empty claim and source quote and a type
// and strength drawn from the enumerations above.
//
// It is the structured-decoding counterpart to the CLAIM/SOURCE_QUOTE text
// format the ledger is written in: an engine that supports response_format
// json_schema can be handed this directly, and internal/gbnf compiles it to a
// GBNF grammar for the local llama.cpp/Ollama path. The verbatim gate is
// unchanged — whatever an engine emits, claims.Parse still checks every quote
// against the source — so this only removes malformed-output drops, never a
// grounding check.
//
// The returned value is a fresh map on every call, safe for the caller to
// mutate or marshal.
func Schema() map[string]any {
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "array",
		"items":   claimObjectSchema(),
	}
}

// claimObjectSchema is the schema of a single claim record. Property order in
// required is the order the grammar emits fields in, and the order the ledger
// writes them: claim, then its source quote, then type and strength.
func claimObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"claim", "source_quote", "type", "strength"},
		"properties": map[string]any{
			"claim":        map[string]any{"type": "string", "minLength": 1},
			"source_quote": map[string]any{"type": "string", "minLength": 1},
			"type":         map[string]any{"type": "string", "enum": toAnySlice(ClaimTypes)},
			"strength":     map[string]any{"type": "string", "enum": toAnySlice(ClaimStrengths)},
		},
	}
}

// toAnySlice copies a []string into an []any so it can sit in a JSON Schema
// map and marshal as a JSON array of strings.
func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
