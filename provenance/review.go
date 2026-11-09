// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package provenance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
	updated, err := withReview(assertions, rv, g.Model)
	if err != nil {
		return nil, Review{}, err
	}
	if top["assertions"], err = json.Marshal(updated); err != nil {
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

// withReview replaces any earlier review with rv and raises the AI
// disclosure to human_validated where it stands, so the other assertions keep
// their bytes and their order.
func withReview(assertions []json.RawMessage, rv Review, model string) ([]json.RawMessage, error) {
	d, err := validated(assertions, model)
	if err != nil {
		return nil, err
	}
	out := withoutLabel(assertions, ReviewLabel)
	disclosure := entry(AIDisclosureLabel, d)
	if i := slices.IndexFunc(out, func(a json.RawMessage) bool { return labelOf(a) == AIDisclosureLabel }); i >= 0 {
		out[i] = disclosure
	} else {
		out = append(out, disclosure)
	}
	return append(out, entry(ReviewLabel, rv)), nil
}

// entry encodes one assertion. Its data is a struct of strings, ints and
// bools, which encoding/json cannot fail on, so there is no error to return.
func entry(label string, data any) json.RawMessage {
	b, _ := json.Marshal(Assertion{Label: label, Data: data})
	return b
}

func labelOf(a json.RawMessage) string {
	var head struct {
		Label string `json:"label"`
	}
	_ = json.Unmarshal(a, &head)
	return head.Label
}

// withoutLabel drops every assertion under label, keeping the others as
// they were written.
func withoutLabel(assertions []json.RawMessage, label string) []json.RawMessage {
	kept := assertions[:0:0]
	for _, a := range assertions {
		if labelOf(a) != label {
			kept = append(kept, a)
		}
	}
	return kept
}
