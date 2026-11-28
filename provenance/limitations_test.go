// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package provenance

import (
	"encoding/json"
	"strings"
	"testing"
)

// The grounding assertion says how many of the source's limitations the
// writer was given and the article carries, and leaves both out when there
// were none, so a manifest from before the fields existed reads the same.
func TestManifestRecordsLimitationCounts(t *testing.T) {
	grounding := func(in ManifestInput) string {
		for _, a := range NewManifest(in).Assertions {
			if a.Label == GroundingLabel {
				b, err := json.Marshal(a.Data)
				if err != nil {
					t.Fatal(err)
				}
				return string(b)
			}
		}
		t.Fatal("no grounding assertion")
		return ""
	}
	with := grounding(ManifestInput{Body: "x", LimitationsGiven: 4, LimitationsCarried: 3})
	if !strings.Contains(with, `"limitations_given":4`) || !strings.Contains(with, `"limitations_carried":3`) {
		t.Errorf("grounding = %s, want the limitation counts", with)
	}
	if without := grounding(ManifestInput{Body: "x"}); strings.Contains(without, "limitations_") {
		t.Errorf("grounding with no limitations = %s, want the fields omitted", without)
	}
}
