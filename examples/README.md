# Examples

Runnable programs demonstrating each part of `draft`. None require a network,
an API token, or a running model — they use in-process data and a demo engine —
so they double as living documentation and smoke tests.

| Example | What it shows | Run |
| ------- | ------------- | --- |
| [`providers`](providers) | The supported session providers and which CLIs are installed | `go run ./examples/providers` |
| [`grounding`](grounding) | Claim verification, the claim ledger, prompt building, and draft validation | `go run ./examples/grounding` |
| [`pipeline`](pipeline) | The full five-phase pipeline end to end against a deterministic demo engine | `go run ./examples/pipeline` |

For the real command — driving a session provider (online) or Ollama (offline)
over your PDFs — see the top-level [README](../README.md).
