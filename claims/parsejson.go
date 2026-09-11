// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"encoding/json"
	"strings"
)

// jsonRecord is the on-the-wire shape of one claim under Schema(): the same
// four fields the text format carries, as JSON keys.
type jsonRecord struct {
	Claim       string `json:"claim"`
	SourceQuote string `json:"source_quote"`
	Type        string `json:"type"`
	Strength    string `json:"strength"`
}

// ParseJSON reads a structured extraction — a JSON array of claim objects
// matching Schema() — and returns the records whose quotes verify, plus the
// count dropped. It is the decoder half of constrained/structured decoding: a
// model constrained to the schema (or to the compiled GBNF grammar) emits this
// shape instead of the CLAIM/SOURCE_QUOTE text, and no malformed block can then
// cost a good claim.
//
// The grounding gate is identical to Parse: every record is checked with
// Verify against the freshly read source, and a near-miss quote gets one
// repair attempt that is re-verified to exactly the same standard. So the wire
// format changes but the guarantee does not — a claim survives only if its
// quote is in the source and every number in it is in that quote.
//
// Malformed JSON yields no records rather than an error: constrained decoding
// makes malformed output impossible, and an unconstrained model that returns
// prose simply grounds nothing, exactly as an empty extraction would.
func ParseJSON(text, source string) (records []Record, dropped int) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, 0
	}
	var raw []jsonRecord
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, 0
	}
	for _, r := range raw {
		rec := Record{
			Claim:       strings.TrimSpace(r.Claim),
			SourceQuote: strings.TrimSpace(r.SourceQuote),
			Type:        strings.TrimSpace(r.Type),
			Strength:    strings.TrimSpace(r.Strength),
		}
		if ok, _ := Verify(rec, source); ok {
			records = append(records, rec)
			continue
		}
		if repaired := repair(rec, source); repaired.SourceQuote != rec.SourceQuote {
			if ok, _ := Verify(repaired, source); ok {
				records = append(records, repaired)
				continue
			}
		}
		dropped++
	}
	return records, dropped
}
