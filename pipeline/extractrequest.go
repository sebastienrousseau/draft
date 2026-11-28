// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/prompt"
)

// extractRequest builds the extraction request for one section as e should
// receive it. With structured set (DRAFT_STRUCTURED_OUTPUT), an engine that
// enforces Request.Schema gets the claim schema and the prompt that asks for
// it. Every other request gets the text prompt.
//
// Structured output is opt-in because the measurement is mixed: on six real
// paper sections gemma3:4b dropped far fewer records in JSON, but answered []
// for one the text prompt found six claims in. The check comes first so the
// default path never sends the schema probe at all.
//
// Either way claims.Parse reads the answer and verifies every quote against
// the section, so the format changes and the grounding gate does not.
func extractRequest(body string, e engine.Engine, structured bool) engine.Request {
	req := engine.Request{Kind: engine.KindExtract, Temperature: extractTemperature}
	if structured && e != nil && engine.SupportsSchema(e) {
		req.Prompt = prompt.ClaimJSON(body)
		req.Schema = claims.Schema()
		return req
	}
	req.Prompt = prompt.Claim(body)
	return req
}

// extractVersion is the prompt version req was built with, for the cache key.
func extractVersion(req engine.Request) string {
	if req.Schema != nil {
		return prompt.ClaimJSONVersion()
	}
	return prompt.ClaimVersion()
}
