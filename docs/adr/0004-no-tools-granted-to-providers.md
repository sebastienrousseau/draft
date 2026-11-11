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

## Amendment, 2026-11: "no tools" was a claim about flags

The consequence above, that the agent "has no tools", did not hold. Not
passing a tool-granting flag only meant draft granted nothing extra: a CLI
left to its defaults still loaded the user's own built-in tools, hooks,
plugins and MCP servers. Asked to, `claude` started with draft's flags read a
file outside its working directory and printed it, so a crafted PDF could have
had it paraphrase `~/.ssh` into a claim. Claim text is not checked against the
source apart from its numbers, so that text could reach the article or the
failed-output file.

What is now guaranteed, and how it was checked (by asking each CLI to read a
canary file outside its working directory):

- **`claude`** runs with `--tools ""`, `--strict-mcp-config` and
  `--safe-mode`: no built-in tools, no MCP servers, no hooks, plugins or
  `CLAUDE.md`. `--bare` would be stricter but reads only `ANTHROPIC_API_KEY`
  and would break the logged-in session.
- **`codex`** runs with its shell and every other built-in tool disabled, the
  user's `config.toml`, rules and project instructions ignored, and a
  permissions profile that denies reads outside the working directory. A
  read-only sandbox alone is not enough: it stops writes, not reads.
- **Every ACP agent** (`copilot`, `claude-acp`, `gemini-acp`, `codex-acp`)
  fails closed: a tool call reported by the agent cancels the turn and
  discards the answer (`ErrToolUse`), because a tool that runs despite draft
  declining every permission request was approved by the agent's own
  configuration. `copilot` also starts without its built-in MCP servers or
  custom instructions; it does try to use tools when asked to, which is why
  the check matters.
- **Every other provider** (`agy`, `cursor-agent`, `grok` and the
  experimental ones) still only gets no tool-granting flag. It runs with
  whatever its own configuration allows.

`TestProvidersKeepTheirRestrictions` pins the restrictions above, and
`TestNoProviderGrantsTools` now also catches `--flag=value` spellings and
flags such as `--permission-mode` whose danger lies in their value.

## Amendment, 2026-11: agy, and providers that keep their tools

`agy`, started with draft's flags and a user setting of
`allowNonWorkspaceAccess`, read the canary file and printed it. It now gets
`--disable-slash-commands` and a `--settings` override for the session alone:
no access outside the workspace, the terminal sandbox on, and the `command`,
`read_file`, `write_file`, `read_url` and `mcp` grants denied. Under them it
tried to read the canary, was refused, and answered `NO-ACCESS`. No setting
removes its web, browser and subagent tools, so the deny rules are not the
whole guarantee: agy's stream reports each tool step, and a turn that starts
one is killed and fails with `ErrToolUse`, as an ACP turn does. `--settings`
rejects a key it does not know, so a renamed setting fails the call instead
of silently dropping a restriction. Checked against agy 1.5.5.

`cursor-agent` and `grok` were not installed where this was done, and no
restriction has been checked against either. Rather than ship flags read
from documentation as a guarantee, both are now experimental: auto mode
never picks a provider that keeps its own tools, and
`TestAutoModeUsesOnlyRestrictedProviders` fails if one becomes auto-selectable
without a pinned, canary-checked restriction. Naming one with `--engine`, or
passing `--experimental`, still runs it, with its own defaults.

`TestProvidersCannotReadOutsideTheSandboxLive` repeats the canary check
against installed CLIs when `DRAFT_LIVE_CANARY` names them.

## Amendment, 2026-11: other tools' credentials

The child environment was filtered only of `DRAFT_*`, so every token in the
user's shell reached every provider. It now also loses credentials, matched by
name (`*_TOKEN`, `*_API_KEY`, `*_SECRET`, `*_PASSWORD`, `AWS_*`, `GH_*`,
`SSH_AUTH_SOCK` and similar), except those the provider signs in with, which
each provider names in its `Credentials`. `claude` keeps `AWS_*` for Bedrock,
and the multi-model CLIs keep every `*_API_KEY`.

This is still not the allowlist rejected above: anything not shaped like a
credential passes, so proxies, locales and provider settings keep working.
`TestProviderGetsOnlyItsOwnCredentials` pins it.
