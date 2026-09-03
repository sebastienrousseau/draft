// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/claims"
)

// The grounding corpus.
//
// The unit tests around Verify check one rule at a time on minimal input. This
// checks the whole gate against realistic sections and realistic model output,
// and pins the numbers that matter: how many candidate claims survive, and how
// many are refused.
//
// It exists because those numbers are the product's actual claim and nothing
// else guards them. A change that loosens verification shows up here as claims
// that should have been dropped surviving; a change that tightens it too far —
// a reworded prompt, a stricter matcher — shows up as recall falling. Both are
// invisible to a test suite that only asks "does Verify reject a bad quote?".
//
// Every case names the rule it guards in `why`. A case that cannot say why it
// exists does not belong here.
const corpusDir = "testdata/corpus"

type corpusCase struct {
	Name     string   `json:"name"`
	Why      string   `json:"why"`
	Verified int      `json:"verified"`
	Dropped  int      `json:"dropped"`
	Survives []string `json:"survives"`
}

func loadCorpus(t *testing.T) []corpusCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(corpusDir, "corpus.json"))
	if err != nil {
		t.Fatalf("reading the corpus manifest: %v", err)
	}
	var m struct {
		Cases []corpusCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing the corpus manifest: %v", err)
	}
	if len(m.Cases) == 0 {
		t.Fatal("the corpus is empty")
	}
	return m.Cases
}

func readCase(t *testing.T, name, suffix string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(corpusDir, name+suffix))
	if err != nil {
		t.Fatalf("case %s: %v", name, err)
	}
	return string(b)
}

func TestGroundingCorpus(t *testing.T) {
	var totalVerified, totalDropped int

	for _, tc := range loadCorpus(t) {
		t.Run(tc.Name, func(t *testing.T) {
			source := readCase(t, tc.Name, ".source.md")
			extraction := readCase(t, tc.Name, ".extraction.md")

			records, dropped := claims.Parse(extraction, source)

			if len(records) != tc.Verified {
				t.Errorf("verified %d claim(s), want %d\nwhy this case exists: %s",
					len(records), tc.Verified, tc.Why)
				for _, r := range records {
					t.Logf("  survived: %s", r.Claim)
				}
			}
			if dropped != tc.Dropped {
				t.Errorf("dropped %d claim(s), want %d\nwhy this case exists: %s",
					dropped, tc.Dropped, tc.Why)
			}

			// Counts alone would pass if the gate kept the wrong claims, so
			// name the ones that must survive.
			for _, want := range tc.Survives {
				var found bool
				for _, r := range records {
					if strings.Contains(r.Claim, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("no surviving claim contains %q\nwhy this case exists: %s", want, tc.Why)
				}
			}

			totalVerified += len(records)
			totalDropped += dropped
		})
	}

	// The recall ratchet. Per-case counts catch a change to one rule; this
	// catches a change that quietly costs a claim or two everywhere at once,
	// which is what a reworded prompt or a stricter matcher actually looks
	// like. Raise it deliberately when the corpus grows; never lower it to
	// make a build pass.
	//
	// A ceiling on `dropped` would say nothing extra: the corpus is fixed, so
	// verified and dropped are complementary and one falling is the other
	// rising. What a second number *can* catch is the corpus itself shrinking
	// — a case file deleted, or a malformed block silently swallowed — so the
	// invariant is on the candidate total instead.
	const (
		minVerifiedAcrossCorpus = 9
		totalCandidateClaims    = 17
	)
	if totalVerified < minVerifiedAcrossCorpus {
		t.Errorf("corpus recall fell: %d claims verified across the corpus, floor is %d",
			totalVerified, minVerifiedAcrossCorpus)
	}
	if got := totalVerified + totalDropped; got != totalCandidateClaims {
		t.Errorf("the corpus offered %d candidate claims, expected %d — a case file "+
			"was removed, or a block is no longer being parsed at all", got, totalCandidateClaims)
	}
	t.Logf("corpus: %d verified, %d dropped, %d candidates across %d cases",
		totalVerified, totalDropped, totalVerified+totalDropped, len(loadCorpus(t)))
}

// De-duplication is what makes the ledger's count honest, so the corpus checks
// it on the case built for it rather than trusting Parse's total.
func TestGroundingCorpusDeduplicates(t *testing.T) {
	source := readCase(t, "12-duplicate-claims", ".source.md")
	extraction := readCase(t, "12-duplicate-claims", ".extraction.md")

	records, _ := claims.Parse(extraction, source)
	if len(records) != 2 {
		t.Fatalf("expected both restatements to verify, got %d", len(records))
	}
	if deduped := claims.Dedupe(records); len(deduped) != 1 {
		t.Errorf("Dedupe left %d record(s), want 1", len(deduped))
	}
}

// A corpus whose cases do not say why they exist rots into a set of numbers
// nobody dares change.
func TestGroundingCorpusCasesAreExplained(t *testing.T) {
	for _, tc := range loadCorpus(t) {
		if len(strings.TrimSpace(tc.Why)) < 40 {
			t.Errorf("case %s has no meaningful `why`", tc.Name)
		}
		for _, suffix := range []string{".source.md", ".extraction.md"} {
			if _, err := os.Stat(filepath.Join(corpusDir, tc.Name+suffix)); err != nil {
				t.Errorf("case %s is missing %s", tc.Name, suffix)
			}
		}
	}
}
