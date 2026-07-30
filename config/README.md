# draft/config

Resolves runtime configuration from command-line flags, environment variables
and defaults, in that order of precedence.

[![Go reference](https://img.shields.io/badge/go.dev-reference-00ADD8?style=flat-square&logo=go&logoColor=white)](https://pkg.go.dev/github.com/sebastienrousseau/draft/config)

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [API](#api)
- [Precedence](#precedence)
- [Nothing fails silently](#nothing-fails-silently)
- [Environment variables](#environment-variables)
- [License](#license)

## Install

```sh
go get github.com/sebastienrousseau/draft@latest
```

```go
import "github.com/sebastienrousseau/draft/config"
```

## Quick start

```go
package main

import (
	"fmt"
	"os"

	"github.com/sebastienrousseau/draft/config"
)

func main() {
	// Environment sits between defaults and flags.
	os.Setenv("DRAFT_NUM_CTX", "2048")

	// Flags are the already-parsed command-line values. A zero value means
	// "not set", so the environment or the default wins for that field.
	cfg := config.Load(config.Flags{Engine: config.EngineOllama})

	fmt.Println(cfg.Engine)        // ollama   — from the flag
	fmt.Println(cfg.ContextLength) // 2048     — from the environment
	fmt.Println(cfg.OllamaModel)   // gemma3:4b — from the default

	// Constructing a Config directly is fine for tests and embedding: nothing
	// in the library requires Load to have run.
	test := config.Config{HomeDir: os.TempDir(), DraftsDir: os.TempDir(), MaxContinue: 3}
	fmt.Println(test.MaxContinue)
}
```

## API

| Symbol | Signature | Purpose |
| ------ | --------- | ------- |
| `Config` | `struct` | The fully-resolved run configuration shared across packages |
| `Flags` | `struct` | Raw command-line values before merging |
| `Load` | `func(flags Flags) Config` | Defaults → environment → flags |
| `EngineAuto` / `EngineOllama` | `const` | Engine-selection sentinels; any other value names a session provider |
| `Default*` | `const` | Every default in one place: models, context length, retries, concurrency, call timeout |
| `OllamaHost` | `const` | `http://127.0.0.1:11434` |
| `FocusBlock` | `const` | `25 * time.Minute`, the dashboard's focus timer |
| `Config.Warnings` | `[]string` | Problems recovered from during resolution; print them |

## Precedence

Flags beat environment variables. Environment variables beat defaults.

`Load` treats a zero-valued `Flags` field as absent, so partially-populated
`Flags` is the normal case — `cmd/draft` passes whatever the user typed and
lets everything else fall through.

## Nothing fails silently

Every numeric variable is clamped at **both** ends — `DRAFT_NUM_CTX` to
[512, 1048576], `DRAFT_EXTRACT_CONCURRENCY` to [1, 32], and so on. A floor
alone leaves a fat-fingered or hostile value free to exhaust memory; a ceiling
alone leaves it free to starve a generation.

When a value is rejected, the default is used **and the reason is appended to
`Config.Warnings`**, which `cmd/draft` prints to stderr. A tunable you believe
took effect but did not is worse than one that was refused outright.

Three resolutions can warn:

| Situation | Behaviour |
| --------- | --------- |
| `os.UserHomeDir` fails (no `HOME` — routine under systemd, cron, containers) | Falls back to the working directory, then to `os.TempDir`. `SourcesDir` and `DraftsDir` are **always absolute**, so drafts can never land in whatever directory the process started in |
| `OLLAMA_HOST` is not a valid `http`/`https` URL | Refused in favour of the default rather than concatenated into a request URL. A bare `host:port` is normalised, not rejected |
| `OLLAMA_HOST` is not loopback | Accepted, but reported — `draft` is documented as working offline, so a remote host means prompts and verbatim source text leave the machine |

`Load` never returns an error: a run should not be blocked by a bad tunable it
can recover from. `Warnings` is how it stays honest about having recovered.

## Environment variables

| Variable | Default | Purpose |
| -------- | ------- | ------- |
| `DRAFT_ENGINE` | `auto` | Backend selection |
| `DRAFT_MODEL_SESSION` | — | Session-provider model override |
| `DRAFT_MODEL` | — | Sets all three Ollama models at once |
| `DRAFT_WRITE_MODEL` | `gemma3:4b` | Ollama writing model |
| `DRAFT_EXTRACT_MODEL` | `gemma3:4b` | Ollama claim-extraction model |
| `DRAFT_EDIT_MODEL` | `gemma3:4b` | Ollama surgical-review model |
| `DRAFT_NUM_CTX` | `8192` | Ollama context window (floor 512) |
| `DRAFT_NUM_PREDICT` | `6000` | Ollama output-token ceiling (floor 1024) |
| `DRAFT_WRITE_RETRIES` | `2` | Rewrite attempts on rule violations |
| `DRAFT_MAX_CONTINUE` | `3` | Continuations on a length-limited stop |
| `DRAFT_EXTRACT_CONCURRENCY` | `4` | Parallel extraction workers (max 32) |
| `DRAFT_CALL_TIMEOUT` | `1800` | Seconds bounding one generation call; `0` disables |
| `DRAFT_EXPERIMENTAL` | — | `1` to let auto use experimental providers |
| `OLLAMA_HOST` | `http://127.0.0.1:11434` | Ollama server address |

`DRAFT_CLAUDE_MODEL` is a deprecated alias for `DRAFT_MODEL_SESSION`, read only
when the latter is unset. `DRAFT_SITE_*` (publisher identity) is read by
[`frontmatter`](../frontmatter), not here.

## License

Licensed under either of [Apache License 2.0](../LICENSE-APACHE) or
[MIT License](../LICENSE-MIT), at your option. © Sebastien Rousseau.

<p align="right"><a href="#draftconfig">Back to top ↑</a></p>
