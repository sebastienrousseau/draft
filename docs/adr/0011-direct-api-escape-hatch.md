# 0011. Add a direct-API escape hatch beside the keyless default

- **Status:** Accepted
- **Date:** 2026-09-11

## Context

[0003](0003-session-cli-backends.md) drives already-authenticated agent CLIs so
no API key is ever read or stored — the core promise. But a machine with no
agent CLI installed and no local Ollama had no online path at all except to go
install one.

## Decision

Add an opt-in `--engine api:<provider>` (`api:anthropic`, `api:openai`) that
calls the hosted chat API directly, reading the key from the provider's own
environment variable and the model from `DRAFT_MODEL`. It is never chosen by
auto mode, and a failed call still falls over to Ollama like any other backend.

## Alternatives considered

- **A general hosted-writing SaaS or a bundled key.** Rejected: that is a
  different product and would break the keyless promise for everyone.
- **No escape hatch at all.** Rejected: it left a bare machine with no online
  option, and users worked around it by installing a CLI purely to satisfy the
  tool — friction with no benefit.
- **Auto-selecting the API when a key is present.** Rejected: presence of a key
  in the environment must not silently change where source text and credentials
  go. The hatch is explicit-only.

## Consequences

- It reintroduces an API key on this one opt-in path, so it is documented as an
  escape hatch and never a default.
- Each provider's request/response shape is maintained in the engine; the seam
  keeps that isolated from the rest of the pipeline.
- The keyless agent-session path remains the default and the recommended one.
