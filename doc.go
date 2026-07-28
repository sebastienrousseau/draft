// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package draft turns research documents into grounded, publication-ready
// Markdown articles.
//
// The module is a command-line tool first — see [cmd/draft] — but every stage
// of its pipeline is an importable package, so the same machinery can be
// driven from your own program.
//
// # The guarantee
//
// Nothing reaches the article that is not traceable to a source. Claims are
// mined from the input, then each one survives only if its quoted evidence
// appears verbatim in that source and every number in the claim appears in
// the quote. The writer is given that verified ledger as its only factual
// substrate, so it arranges pre-checked facts rather than inventing new ones.
// The finished draft is then checked against the house rules — structure,
// length, banned vocabulary, faithfulness — and a violation triggers a
// targeted rewrite rather than a silent pass.
//
// # The packages
//
//   - [github.com/sebastienrousseau/draft/config]: flag, environment, and
//     default resolution.
//   - [github.com/sebastienrousseau/draft/engine]: the Engine seam. A session
//     provider (an installed AI coding-agent CLI, driven through its own
//     login) or a local Ollama model sit behind the same interface, along
//     with the fallback chain between them.
//   - [github.com/sebastienrousseau/draft/prompt]: the claim-extraction,
//     writing, and review briefs.
//   - [github.com/sebastienrousseau/draft/claims]: claim parsing, verbatim
//     verification, and ledger rendering.
//   - [github.com/sebastienrousseau/draft/rules]: the shared editorial
//     constants — banned vocabulary, length bands, structural markers.
//   - [github.com/sebastienrousseau/draft/validate]: house-rule and
//     faithfulness checks over a finished draft.
//   - [github.com/sebastienrousseau/draft/frontmatter]: metadata extraction,
//     YAML generation, and identity-preserving regeneration of an article set.
//   - [github.com/sebastienrousseau/draft/pipeline]: the orchestration that
//     joins them, including retries, continuation past a truncated stop, and
//     the provider fallback chain.
//
// # Stability
//
// While the module is at 0.0.x the exported Go API may change between
// releases without a deprecation cycle; pin an exact version if you depend on
// it. The command-line surface is the stable one. Breaking changes are always
// recorded in CHANGELOG.md.
//
// # Getting started
//
// The examples directory holds a runnable, network-free demo of each
// capability — including the full pipeline and the dashboard — none of which
// need a model, an API key, or a network connection.
package draft
