// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import "testing"

const labelSource = "The trial enrolled adults only. Results were mixed across sites."

// The extraction prompt tells the model to write "none" when a field does not
// apply; doing as asked for TYPE must not cost the claim. STRENGTH "none"
// stays a drop: an empty strength reaches the writer as no hedge at all.
func TestNoneTypeMeansNotApplicable(t *testing.T) {
	text := "CLAIM: The trial enrolled adults only.\nSOURCE_QUOTE: \"The trial enrolled adults only.\"\nTYPE: None\nSTRENGTH: demonstrated\n---"
	records, _ := Parse(text, labelSource)
	if len(records) != 1 || records[0].Type != "" {
		t.Fatalf("text path: %+v", records)
	}
	js := `[{"claim":"The trial enrolled adults only.","source_quote":"The trial enrolled adults only.","type":"NONE","strength":"hedged"}]`
	jr, _ := ParseJSON(js, labelSource)
	if len(jr) != 1 || jr[0].Type != "" {
		t.Fatalf("JSON path: %+v", jr)
	}
	js = `[{"claim":"The trial enrolled adults only.","source_quote":"The trial enrolled adults only.","type":"result","strength":"none"}]`
	if jr, dropped := ParseJSON(js, labelSource); len(jr) != 0 || dropped != 1 {
		t.Fatalf("JSON STRENGTH none must drop: %+v dropped=%d", jr, dropped)
	}
	text = "CLAIM: The trial enrolled adults only.\nSOURCE_QUOTE: \"The trial enrolled adults only.\"\nTYPE: nonesuch\nSTRENGTH: demonstrated\n---"
	if records, dropped := Parse(text, labelSource); len(records) != 0 || dropped != 1 {
		t.Fatalf("a TYPE that only starts with none must still be checked: %+v dropped=%d", records, dropped)
	}
}
