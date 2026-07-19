# Security Policy

## Supported Versions

| Version | Supported |
|:--------|:---------:|
| 0.0.x   | Yes       |

## Reporting a Vulnerability

Report security vulnerabilities by opening a
[private advisory](https://github.com/sebastienrousseau/draft/security/advisories/new)
or by emailing **sebastian.rousseau@gmail.com**.

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
  agent CLI (`claude`, `copilot`, …); `draft` never reads, stores, or logs an
  API key.
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

Online, `draft` drives an AI coding-agent CLI in its non-interactive mode, some
of which auto-approve tool use (for example `copilot --allow-all-tools`). `draft`
asks only for text and treats your sources as untrusted, but you are still
handing a research document to an agent that *can* act on your machine. Treat
sources as you would any untrusted input, and prefer the offline Ollama backend
(`--engine ollama`) for material you do not trust.
