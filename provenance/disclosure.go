// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package provenance

import (
	"encoding/json"
	"errors"
	"fmt"
)

// The EU AI Act's Art. 50(2) asks for AI-generated text to be marked in a
// machine-readable way. The created action's digitalSourceType already says a
// trained model made the text; C2PA 2.4's AI Disclosure assertion (§18.28)
// adds what that cannot say: how much a person was involved. draft records
// prompt_guided, because a person chose the sources and ran it but did not
// approve the result, and a review attested with --attest raises it to
// human_validated.

// AIDisclosureLabel is the C2PA 2.4 AI Disclosure assertion's label.
const AIDisclosureLabel = "c2pa.ai-disclosure"

// The humanOversightLevel values draft writes. The spec's third,
// fully_autonomous, does not apply: draft never runs without a person
// choosing what it reads.
const (
	OversightPromptGuided   = "prompt_guided"
	OversightHumanValidated = "human_validated"
)

// genericModelType is the spec's model type for a model not described by a
// framework-specific one. draft drives whatever model its provider runs, so it
// cannot name a framework.
const genericModelType = "c2pa.types.model"

// AIDisclosure is the data of the c2pa.ai-disclosure assertion, limited to the
// fields the 2.4 schema has settled; the spec lists the rest as pending.
type AIDisclosure struct {
	ModelType      string         `json:"modelType"`
	ModelName      string         `json:"modelName,omitempty"`
	ContentProfile ContentProfile `json:"contentProfile"`
}

// ContentProfile carries the declared degree of human involvement.
type ContentProfile struct {
	HumanOversightLevel string `json:"humanOversightLevel"`
}

func newAIDisclosure(model string) AIDisclosure {
	return AIDisclosure{
		ModelType:      genericModelType,
		ModelName:      model,
		ContentProfile: ContentProfile{HumanOversightLevel: OversightPromptGuided},
	}
}

// validated returns the manifest's disclosure raised to human_validated, or a
// new one for a manifest written before draft emitted it. A disclosure that
// cannot be read is an error rather than replaced, so a review never papers
// over a manifest someone has damaged.
func validated(assertions []json.RawMessage, model string) (AIDisclosure, error) {
	d := newAIDisclosure(model)
	for _, a := range assertions {
		var entry struct {
			Label string          `json:"label"`
			Data  json.RawMessage `json:"data"`
		}
		if json.Unmarshal(a, &entry) != nil || entry.Label != AIDisclosureLabel {
			continue
		}
		if err := json.Unmarshal(entry.Data, &d); err != nil {
			return d, fmt.Errorf("malformed %s assertion: %w", AIDisclosureLabel, err)
		}
	}
	d.ContentProfile.HumanOversightLevel = OversightHumanValidated
	return d, nil
}

// errUnbackedValidation is the problem a manifest has when its disclosure
// claims a person approved the text but records no review saying who.
var errUnbackedValidation = errors.New("the AI disclosure says a person validated the text, but no review is recorded")

// readDisclosure fills r's SourceType from the created action, its Oversight
// from the AI disclosure and its Review from the review assertion. Any may be
// absent; a malformed review or disclosure is an error, because a statement
// that cannot be read must not pass as no statement.
func (r *CheckReport) readDisclosure(manifest []byte) error {
	var mf manifestFile
	if err := json.Unmarshal(manifest, &mf); err != nil {
		return err
	}
	for _, a := range mf.Assertions {
		if err := r.readAssertion(a.Label, a.Data); err != nil {
			return err
		}
	}
	if r.Oversight == OversightHumanValidated && r.Review == nil {
		return errUnbackedValidation
	}
	return nil
}

func (r *CheckReport) readAssertion(label string, data json.RawMessage) error {
	switch label {
	case "c2pa.actions":
		r.SourceType = createdSourceType(data)
	case AIDisclosureLabel:
		var d AIDisclosure
		if err := json.Unmarshal(data, &d); err != nil {
			return fmt.Errorf("malformed %s assertion: %w", AIDisclosureLabel, err)
		}
		r.Oversight = d.ContentProfile.HumanOversightLevel
	case ReviewLabel:
		var rv Review
		if err := json.Unmarshal(data, &rv); err != nil {
			return fmt.Errorf("malformed review assertion: %w", err)
		}
		r.Review = &rv
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
