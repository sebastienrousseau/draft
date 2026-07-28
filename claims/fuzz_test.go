// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"strings"
	"testing"
)

// FuzzParse drives the grounding gate with arbitrary model output. The
// security-critical invariant is that a surviving claim's quote must appear
// verbatim in the source: everything downstream treats the ledger as fact, so
// a quote that is not in the source is a fabrication that reached the writer.
func FuzzParse(f *testing.F) {
	f.Add("CLAIM: A scored 0.82\nSOURCE_QUOTE: \"scored 0.82\"\nTYPE: metric\nSTRENGTH: demonstrated\n---",
		"The system scored 0.82 on the held-out set.")
	f.Add("NONE", "some source")
	f.Add("", "")
	f.Add("CLAIM: x\nSOURCE_QUOTE: \"not present\"\nTYPE: result\nSTRENGTH: claimed\n---", "unrelated text")
	f.Add("---\n---\n---", "s")
	f.Add("CLAIM: 5x faster\nSOURCE_QUOTE: \"5x\"\nTYPE: metric\nSTRENGTH: demonstrated\n---", "it was 5x")

	f.Fuzz(func(t *testing.T, text, source string) {
		records, dropped := Parse(text, source)
		if dropped < 0 {
			t.Fatalf("negative dropped count %d", dropped)
		}
		for _, r := range records {
			if r.SourceQuote == "" {
				t.Errorf("verified record has an empty quote: %+v", r)
			}
			if !strings.Contains(source, r.SourceQuote) {
				t.Errorf("ungrounded quote survived verification:\nquote:  %q\nsource: %q", r.SourceQuote, source)
			}
			if r.Claim == "" {
				t.Errorf("verified record has an empty claim: %+v", r)
			}
		}
	})
}
