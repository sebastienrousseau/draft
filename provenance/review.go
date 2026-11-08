// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package provenance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// A person who reviews an article and takes editorial responsibility for it
// is what the EU AI Act's Art. 50(4) exemption for AI-generated text turns
// on, and a reviewer usually edits. The review assertion records who
// reviewed, when, and the digest of exactly the text they approved, so
// --verify can tell "edited and then reviewed" from "edited since".

// ReviewLabel is the assertion label a human review sits under.
const ReviewLabel = "com.draftlib.review"

// Review is the data of the com.draftlib.review assertion.
type Review struct {
	Schema   int    `json:"schema"`
	Reviewer string `json:"reviewer"`
	When     string `json:"when"`
	// ArticleSHA256 is the digest of the text the reviewer approved, which
	// may differ from the grounding assertion's article_sha256.
	ArticleSHA256 string `json:"article_sha256"`
	// Edited records that the approved text is not the text draft wrote, so
	// the grounding figures describe draft's version, not this one.
	Edited bool `json:"edited"`
}

// Attest returns manifest with a review of body by reviewer at when, and the
// review itself. An earlier review is replaced, not kept beside the new one:
// a review answers for the text as it now stands, and only one text stands.
// The rest of the manifest is carried over unchanged.
func Attest(manifest []byte, body, reviewer string, when time.Time) ([]byte, Review, error) {
	reviewer = strings.TrimSpace(reviewer)
	if reviewer == "" {
		return nil, Review{}, errors.New("a review needs a reviewer")
	}
	g, _, err := ParseManifest(manifest)
	if err != nil {
		return nil, Review{}, err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(manifest, &top); err != nil {
		return nil, Review{}, fmt.Errorf("not a draft manifest: %w", err)
	}
	var assertions []json.RawMessage
	if err := json.Unmarshal(top["assertions"], &assertions); err != nil {
		return nil, Review{}, fmt.Errorf("malformed assertions: %w", err)
	}
	sum := sha256.Sum256([]byte(body))
	rv := Review{
		Schema:        Schema,
		Reviewer:      reviewer,
		When:          when.UTC().Format(time.RFC3339),
		ArticleSHA256: hex.EncodeToString(sum[:]),
	}
	rv.Edited = rv.ArticleSHA256 != g.ArticleSHA256
	kept := withoutLabel(assertions, ReviewLabel)
	entry, err := json.Marshal(Assertion{Label: ReviewLabel, Data: rv})
	if err != nil {
		return nil, Review{}, err
	}
	if top["assertions"], err = json.Marshal(append(kept, entry)); err != nil {
		return nil, Review{}, err
	}
	out, err := json.MarshalIndent(top, "", "  ")
	return out, rv, err
}

// covers reports whether rv approved the text with digest sum. No review
// covers anything.
func (rv *Review) covers(sum string) bool {
	return rv != nil && rv.ArticleSHA256 == sum
}

// withoutLabel drops every assertion under label, keeping the others as
// they were written.
func withoutLabel(assertions []json.RawMessage, label string) []json.RawMessage {
	kept := assertions[:0:0]
	for _, a := range assertions {
		var head struct {
			Label string `json:"label"`
		}
		if json.Unmarshal(a, &head) == nil && head.Label == label {
			continue
		}
		kept = append(kept, a)
	}
	return kept
}

// readDisclosure fills r's SourceType from the created action and its Review
// from the review assertion. Either may be absent; a malformed review is an
// error, because a review that cannot be read must not pass as no review.
func (r *CheckReport) readDisclosure(manifest []byte) error {
	var mf manifestFile
	if err := json.Unmarshal(manifest, &mf); err != nil {
		return err
	}
	for _, a := range mf.Assertions {
		switch a.Label {
		case "c2pa.actions":
			r.SourceType = createdSourceType(a.Data)
		case ReviewLabel:
			var rv Review
			if err := json.Unmarshal(a.Data, &rv); err != nil {
				return fmt.Errorf("malformed review assertion: %w", err)
			}
			r.Review = &rv
		}
	}
	return nil
}

// createdSourceType returns the digitalSourceType of the c2pa.created action.
func createdSourceType(data json.RawMessage) string {
	var actions struct {
		Actions []struct {
			Action            string `json:"action"`
			DigitalSourceType string `json:"digitalSourceType"`
		} `json:"actions"`
	}
	if json.Unmarshal(data, &actions) != nil {
		return ""
	}
	for _, a := range actions.Actions {
		if a.Action == "c2pa.created" {
			return a.DigitalSourceType
		}
	}
	return ""
}

// AIGenerated reports whether a digital source type says a trained model
// made the content.
func AIGenerated(sourceType string) bool {
	return sourceType == digitalSourceType
}
