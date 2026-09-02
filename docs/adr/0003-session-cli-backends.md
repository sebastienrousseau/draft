# 0003. Drive already-authenticated agent CLIs instead of handling API keys

- **Status:** Accepted
- **Date:** 2026-07

## Context

Writing an article well needs a capable model. The obvious route is an API key
per provider, which means the tool stores, reads and logs credentials, and the
user pays per token on top of subscriptions they already hold.

Most target users already have at least one agent CLI installed and logged in.

## Decision

Invoke those CLIs in headless mode and let each authenticate through its own
session. `draft` never reads, stores or logs an API key. When no CLI is
available or a call fails, fall back to a local Ollama model.

## Alternatives considered

- **Direct provider APIs.** Requires credential handling and per-token billing,
  and one HTTP client per provider.
- **Ollama only.** Removes the dependency entirely, but a 4B local model writes
  visibly worse prose than a frontier model, and quality is the point.
- **A protocol client (ACP).** The right long-term answer, and now standardised
  with a registry — but it did not exist in usable form when this was decided.

## Consequences

- No credentials in the tool, and no marginal cost to the user.
- **A hand-maintained invocation table** for ten CLIs, each of which can break
  the integration with a flag rename. Four entries are marked experimental
  because their output was never verified end to end.
- One cold subprocess per model call, with no session or prefix-cache reuse:
  measured warm startup ranges from 0.10 s (`claude`) to 8.53 s (`copilot`),
  the latter costing ~128 s per paper before a token is generated.
- Feeding a third-party document to an agent on the user's machine created the
  exposure that [ADR 0004](0004-no-tools-granted-to-providers.md) exists to
  bound.
- Migrating to ACP would replace the table with one protocol client and enable
  a long-lived session. It is the main open architectural item.
