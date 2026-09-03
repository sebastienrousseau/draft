# 0004. Grant provider subprocesses no tools, and run them in an empty directory

- **Status:** Accepted
- **Date:** 2026-09

## Context

[ADR 0003](0003-session-cli-backends.md) means `draft` puts the text of a file
the user downloaded from the internet into a prompt for an agent running on
their machine, with their credentials. That is the canonical
indirect-prompt-injection shape, and CSA's 2026 research names PDFs explicitly
as an in-the-wild vector — the shift from theoretical to operational happened
once agentic CLIs began executing shell commands rather than emitting
suggestions.

An audit found four things had composed into a live path:

1. The extraction prompt appended source text with no delimiter and no
   statement that it was data.
2. No `cmd.Dir` or `cmd.Env` anywhere, so every provider inherited the user's
   working directory and full environment.
3. `copilot` was invoked with `--allow-all-tools`; `cursor-agent` with
   `--force`, which its own help documents as an alias for `--yolo`.
4. Default `auto` mode could select either without the user choosing it.

A crafted claim would be dropped by the grounding gate — but the tool call has
already happened by then.

## Decision

Bound the blast radius rather than trying to win an adversarial text game:

- No provider is invoked with a tool-granting flag. A test pins the table
  against a list of known ones.
- Each call runs in a fresh empty temporary directory, removed on return.
  Failure to create it fails the call rather than falling back to the CWD.
- `DRAFT_*` is stripped from the child environment.
- Source text, the ledger and the draft under review are fenced in a
  nonce-delimited block with an explicit data-not-instructions instruction.

## Alternatives considered

- **Fencing alone.** No prompt-level measure wins outright; it is defence in
  depth, not the control.
- **A strict environment allowlist.** Rejected: every provider finds its
  credentials somewhere different, so a wrong list breaks authentication and
  surfaces as a confusing provider error rather than a security decision.
- **Refusing untrusted sources.** Every source is untrusted. That is the tool.
- **Ollama only for untrusted material.** Still the right advice for genuinely
  hostile input, and the README says so — but it cannot be the only answer,
  because users will not classify their inputs.

## Consequences

- A crafted document reaches an agent that has no tools, no project context and
  an empty working directory. It can still influence *text*; it cannot act.
- Not loading project agent configuration also removes MCP server startup from
  every one of the dozen-plus calls a paper costs.
- `copilot` and `cursor-agent` lost flags they were previously given. This only
  reduces privilege, but both need re-verifying end to end.
- The ledger is fenced too, because a `SOURCE_QUOTE` is verbatim source by
  construction — whoever controls the PDF controls what reaches the writer.
