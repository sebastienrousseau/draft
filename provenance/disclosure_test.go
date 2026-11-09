// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package provenance

import (
	"encoding/json"
	"strings"
	"testing"
)

func labels(t *testing.T, manifest []byte) []string {
	t.Helper()
	var mf manifestFile
	if err := json.Unmarshal(manifest, &mf); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, a := range mf.Assertions {
		out = append(out, a.Label)
	}
	return out
}

// A new article declares prompt_guided; attesting raises it to
// human_validated in place, keeping the model name and every other
// assertion where it was.
func TestAttestRaisesTheOversightLevel(t *testing.T) {
	m := testManifest(t, "x\n")
	if r := CheckArticle("x\n", m, nil, nil); r.Oversight != OversightPromptGuided {
		t.Fatalf("new manifest oversight = %q", r.Oversight)
	}
	out, _, err := Attest(m, "x\n", "Ada", reviewTime)
	if err != nil {
		t.Fatal(err)
	}
	r := CheckArticle("x\n", out, nil, nil)
	if r.Oversight != OversightHumanValidated || !r.OK() {
		t.Errorf("after attest: oversight %q, problems %q", r.Oversight, r.Problems)
	}
	want := []string{"c2pa.actions", AIDisclosureLabel, GroundingLabel, ReviewLabel}
	if got := labels(t, out); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("assertions = %v, want %v", got, want)
	}
	if rec := NewVerificationRecord(r, nil); rec.Disclosure.HumanOversightLevel != OversightHumanValidated {
		t.Errorf("record disclosure = %+v", rec.Disclosure)
	}
}

// A manifest written before draft emitted the disclosure gains one when it
// is reviewed, named after the grounding's model.
func TestAttestAddsADisclosureToAnOlderManifest(t *testing.T) {
	m := NewManifest(ManifestInput{Body: "x\n", Model: "sonnet"})
	m.Assertions = []Assertion{m.Assertions[0], m.Assertions[2]}
	old, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if r := CheckArticle("x\n", old, nil, nil); r.Oversight != "" || !r.OK() {
		t.Fatalf("older manifest: %+v", r)
	}
	out, _, err := Attest(old, "x\n", "Ada", reviewTime)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"modelName": "sonnet"`) || CheckArticle("x\n", out, nil, nil).Oversight != OversightHumanValidated {
		t.Errorf("added disclosure:\n%s", out)
	}
}

// A statement that cannot be read, or one that claims more than the
// manifest records, is a problem, never a silent pass.
func TestDisclosureProblems(t *testing.T) {
	m := string(testManifest(t, "x\n"))
	damaged := strings.Replace(m, `"contentProfile": {`, `"contentProfile": "no", "x": {`, 1)
	if _, _, err := Attest([]byte(damaged), "x\n", "Ada", reviewTime); err == nil || !strings.Contains(err.Error(), "malformed c2pa.ai-disclosure") {
		t.Errorf("attest over a damaged disclosure: %v", err)
	}
	if r := CheckArticle("x\n", []byte(damaged), nil, nil); r.OK() || len(r.Problems) != 1 || !strings.Contains(r.Problems[0], "malformed c2pa.ai-disclosure") {
		t.Errorf("damaged disclosure: %q", r.Problems)
	}

	claimed := strings.Replace(m, OversightPromptGuided, OversightHumanValidated, 1)
	if r := CheckArticle("x\n", []byte(claimed), nil, nil); r.OK() || len(r.Problems) != 1 || !strings.Contains(r.Problems[0], "no review is recorded") {
		t.Errorf("validation with no review: %q", r.Problems)
	}
}
