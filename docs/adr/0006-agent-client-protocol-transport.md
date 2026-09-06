# 0006. Add an Agent Client Protocol transport beside the one-shot CLIs

- **Status:** Accepted
- **Date:** 2026-09

## Context

Every backend was a hand-coded headless invocation of one CLI, derived from
its `--help`, with the prompt on stdin or in argv and the answer scraped from
stdout. Ten of them, four never verified end to end. A declined prompt looked
like a crash: the claude CLI exits 1 with an empty stderr, and until 0.0.34
that demoted the provider for the rest of the queue.

The Agent Client Protocol is the standard that exists to end that shape:
JSON-RPC 2.0 over the agent's stdio, `initialize`, `session/new`,
`session/prompt`, streamed `session/update` notifications, and a
`stopReason` on every turn that names `refusal` and `max_tokens` outright.

## Decision

Add `engine.ACP`: one agent process per engine for the life of the run, a new
session for every `Generate`, no session reuse. Register `claude-acp` as a
stable provider and `gemini-acp` and `codex-acp` as experimental. The agent's
own requests to the client — permission for a tool, a file read — are always
declined, in line with [ADR 0004](0004-no-tools-granted-to-providers.md).

## Alternatives considered

- **Reuse one session across prompts.** Keeps the agent warm but carries
  conversation state from section to section, so the claims for section 9
  can be coloured by section 3. That breaks the one invariant the product
  rests on. Rejected.
- **`claude -p --input-format stream-json` as a long-lived process.** The same
  accumulation problem, and bespoke to one CLI.
- **Replace the one-shot table with ACP entirely.** Only three agents speak it
  today and only one is verified; the table stays, ACP sits beside it.

## Consequences

- A refusal is a typed outcome. Combined with `engine.ErrRefused` from the
  one-shot path, the pipeline can route a declined prompt instead of
  demoting the engine.
- No wall-clock win. Measured against `claude-code-acp` 0.16 on 2026-09-06: a
  warm second session plus prompt took 4.15 s against 4.33 s for a cold
  `claude -p`, because the adapter starts an agent per session. The README
  says so; nothing here claims otherwise.
- The runner now owns a process. `Runner.Close` releases it, and the CLI
  defers that at exit.
