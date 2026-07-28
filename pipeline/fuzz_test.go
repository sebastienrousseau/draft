// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"strings"
	"testing"
)

// FuzzParseSurgicalEdits drives the review-mode edit parser with arbitrary
// model output. The parser reads untrusted JSON that is then applied to a
// finished article, so it must reject anything malformed rather than panic,
// and an accepted edit set must never silently corrupt the draft.
func FuzzParseSurgicalEdits(f *testing.F) {
	f.Add(`[{"find":"a","replace":"b","reason":"generic"}]`, "a source text")
	f.Add(`prelude [{"find":"quick","replace":"slow","reason":"filler"}] trailing`, "the quick fox")
	f.Add("<think>reasoning</think>[]", "unchanged")
	f.Add("not json at all", "src")
	f.Add(`[{"find":"","replace":"x","reason":"generic"}]`, "src")
	f.Add(`[{"find":"a","replace":"b","reason":"unsupported"}]`, "a")

	f.Fuzz(func(t *testing.T, response, source string) {
		edits, err := parseSurgicalEdits(response)
		if err != nil {
			return // rejecting malformed output is the correct behaviour
		}
		out, applyErr := applySurgicalEdits(source, edits)
		if applyErr != nil {
			return // refusing to apply is always safe
		}
		// An accepted application must not destroy the draft.
		if source != "" && out == "" && len(edits) > 0 {
			allEmpty := true
			for _, e := range edits {
				if e.Replace != "" {
					allEmpty = false
				}
			}
			if !allEmpty {
				t.Errorf("edits emptied a non-empty draft: source=%q edits=%+v", source, edits)
			}
		}
		// With no edits the draft must come back untouched.
		if len(edits) == 0 && out != source {
			t.Errorf("empty edit set changed the draft:\nbefore: %q\nafter:  %q", source, out)
		}
		// Every applied edit had to match exactly once, so its replacement
		// must be present in the result.
		for _, e := range edits {
			if e.Replace != "" && !strings.Contains(out, e.Replace) {
				t.Errorf("replacement %q missing from applied output %q", e.Replace, out)
			}
		}
	})
}
