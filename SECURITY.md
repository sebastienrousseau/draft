# Security Policy

## Supported Versions

| Version | Supported |
| :------ | :-------: |
| 0.0.x   | Yes       |

## Reporting a Vulnerability

Report security vulnerabilities by opening a
[private advisory](https://github.com/sebastienrousseau/draft/security/advisories/new)
or by emailing **<sebastian.rousseau@gmail.com>**.

Do not open a public issue for security reports.

Include:

- A description of the vulnerability.
- Steps to reproduce.
- Affected versions.
- Any suggested fix (optional).

Expect an initial response within 48 hours. A fix or mitigation plan will follow
within 7 days of confirmation.

## Security Design

`draft` is a local CLI that orchestrates other tools. Its posture:

- **No tokens on disk.** Session backends shell out to an already-authenticated
  agent CLI (`claude`, `copilot`, …) and never read, store or log an API key.
  The opt-in `--engine api:<provider>` escape hatch is the one exception: it
  reads the provider's own key from the environment ([ADR
  0011](docs/adr/0011-direct-api-escape-hatch.md)), and is never chosen
  automatically.
- **No shell interpolation.** External tools (`pdftotext`, `textutil`, the
  provider CLIs, Ollama) are invoked with argument vectors via `os/exec` and
  `net/http`, never through a shell, and always under a cancellable context with
  timeouts.
- **Prompt-injection aware.** Template and source text are quoted to the model as
  untrusted evidence, and the writing prompt explicitly instructs it to ignore
  any instructions found inside them.
- **Grounding as a control.** Every fact must trace to a verbatim-quote-verified
  claim; ungrounded numbers and silent metric conversions are flagged and
  unverifiable claims are dropped before writing.

## Agent Trust Surface

Online, `draft` drives an AI coding-agent CLI in its non-interactive mode and
asks it only for text. What that agent can do depends on the provider:

- `claude` and `codex` are started with no tools at all, and none of your own
  hooks, plugins or MCP servers.
- An agent driven over ACP (`copilot`, `claude-acp`, …) is refused every
  permission it asks for, and a turn in which it runs a tool anyway is
  cancelled and its answer thrown away.
- Any other provider is given no tool-granting flag, but otherwise runs with
  whatever its own configuration allows.

Each runs in an empty temporary directory. Its environment loses `draft`'s own
`DRAFT_*` settings and every credential but its own: variables ending
`_TOKEN`, `_API_KEY`, `_SECRET`, `_PASSWORD` and the like, `AWS_*`, `GH_*` and
`SSH_AUTH_SOCK` are withheld unless the provider signs in with them. That list
is matched by name, so a secret under a name it does not cover still gets
through.
[ADR 0004](docs/adr/0004-no-tools-granted-to-providers.md) records what is
guaranteed and how it was checked. Treat sources as you would any untrusted
input, and prefer the offline Ollama backend (`--engine ollama`) for material
you do not trust.
