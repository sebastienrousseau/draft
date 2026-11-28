// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/prompt"
)

// extractRequest builds the extraction request for one section as e should
// receive it. An engine that enforces Request.Schema gets the claim schema
// and the prompt that asks for it, so a small local model cannot lose a good
// claim to a malformed CLAIM: block. Every other engine gets the text prompt.
//
// Either way claims.Parse reads the answer and verifies every quote against
// the section, so the format changes and the grounding gate does not.
func extractRequest(body string, e engine.Engine) engine.Request {
	req := engine.Request{Kind: engine.KindExtract, Temperature: extractTemperature}
	if e != nil && engine.SupportsSchema(e) {
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
