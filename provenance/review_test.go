// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package provenance

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testManifest(t *testing.T, body string) []byte {
	t.Helper()
	m := NewManifest(ManifestInput{Title: "t", Body: body, Version: "0.0.44", Sources: []Source{{Path: "/s/a.pdf", SHA256: "aa"}}})
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var reviewTime = time.Date(2026, 11, 8, 10, 0, 0, 0, time.FixedZone("CET", 3600))

func TestAttestRecordsTheReviewedTextAndKeepsTheRest(t *testing.T) {
	m := testManifest(t, "original\n")
	out, rv, err := Attest(m, "edited\n", "  Ada  ", reviewTime)
	if err != nil {
		t.Fatal(err)
	}
	if rv.Reviewer != "Ada" || rv.When != "2026-11-08T09:00:00Z" || !rv.Edited || rv.Schema != Schema {
		t.Errorf("review = %+v", rv)
	}
	r := CheckArticle("edited\n", out, nil, nil)
	if !r.ReviewMatches || r.DigestMatches || !r.OK() || r.Review.Reviewer != "Ada" {
		t.Errorf("check = %+v", r)
	}
	if !AIGenerated(r.SourceType) || len(r.Sources) != 1 || r.Generator.Version != "0.0.44" {
		t.Errorf("attesting lost part of the manifest: %+v", r)
	}

	// Unedited text, reviewed twice: one review, the latest.
	out, rv, _ = Attest(m, "original\n", "Ada", reviewTime)
	if rv.Edited {
		t.Error("a review of draft's own text is not an edit")
	}
	out, _, _ = Attest(out, "original\n", "Bo", reviewTime)
	if n := strings.Count(string(out), ReviewLabel); n != 1 {
		t.Errorf("%d review assertions after re-attesting, want 1", n)
	}
	if r := CheckArticle("original\n", out, nil, nil); r.Review.Reviewer != "Bo" || !r.OK() {
		t.Errorf("latest review = %+v", r.Review)
	}
}

// Without a matching review, an edited article still fails: the review is
// the only thing that can account for an edit.
func TestAnEditWithoutAMatchingReviewFails(t *testing.T) {
	m := testManifest(t, "original\n")
	if CheckArticle("edited\n", m, nil, nil).OK() {
		t.Error("an unreviewed edit passed")
	}
	reviewed, _, _ := Attest(m, "edited\n", "Ada", reviewTime)
	if r := CheckArticle("edited again\n", reviewed, nil, nil); r.OK() || r.ReviewMatches {
		t.Errorf("an edit after the review passed: %+v", r)
	}
}

func TestAttestRefusals(t *testing.T) {
	m := testManifest(t, "x\n")
	for _, tc := range []struct {
		name, manifest, reviewer, want string
	}{
		{"no reviewer", string(m), " ", "needs a reviewer"},
		{"not JSON", "nope", "Ada", "not a draft manifest"},
		{"no grounding", `{"assertions":[]}`, "Ada", "no com.draftlib.grounding"},
		{"assertions not a list", `{"assertions":{"label":"com.draftlib.grounding","data":{}}}`, "Ada", "not a draft manifest"},
	} {
		if _, _, err := Attest([]byte(tc.manifest), "x\n", tc.reviewer, reviewTime); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
	}
}

// A review that cannot be read must not pass as no review at all.
func TestAMalformedReviewIsAProblem(t *testing.T) {
	m := strings.Replace(string(testManifest(t, "x\n")), `"assertions": [`, `"assertions": [{"label":"`+ReviewLabel+`","data":"not an object"},`, 1)
	r := CheckArticle("x\n", []byte(m), nil, nil)
	if r.OK() || len(r.Problems) != 1 || !strings.Contains(r.Problems[0], "malformed review") {
		t.Errorf("problems = %q", r.Problems)
	}
}

func TestDisclosureReading(t *testing.T) {
	if got := createdSourceType(json.RawMessage(`"junk"`)); got != "" {
		t.Errorf("malformed actions = %q", got)
	}
	if got := createdSourceType(json.RawMessage(`{"actions":[{"action":"c2pa.edited","digitalSourceType":"x"}]}`)); got != "" {
		t.Errorf("no created action = %q", got)
	}
	if AIGenerated("http://cv.iptc.org/newscodes/digitalsourcetype/digitalCapture") || !AIGenerated(digitalSourceType) {
		t.Error("AIGenerated misreads the IPTC term")
	}
	if err := new(CheckReport).readDisclosure([]byte("nope")); err == nil {
		t.Error("readDisclosure accepted non-JSON")
	}
}

func TestRecordCarriesDisclosureAndReview(t *testing.T) {
	m := testManifest(t, "original\n")
	reviewed, _, _ := Attest(m, "edited\n", "Ada", reviewTime)
	rec := NewVerificationRecord(CheckArticle("edited\n", reviewed, nil, nil), nil)
	if !rec.Verified || !rec.Disclosure.AIGenerated || rec.Disclosure.DigitalSourceType != digitalSourceType {
		t.Errorf("record = %+v", rec)
	}
	if rv := rec.Review; rv == nil || rv.Reviewer != "Ada" || !rv.Matches || !rv.Edited || rv.SHA256 == "" {
		t.Errorf("review = %+v", rec.Review)
	}
	if rec := NewVerificationRecord(CheckArticle("original\n", m, nil, nil), nil); rec.Review != nil {
		t.Error("an unreviewed article has a review in its record")
	}
}
