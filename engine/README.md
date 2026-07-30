# draft/engine

Text generation over interchangeable backends, behind a two-method interface.

[![Go reference](https://img.shields.io/badge/go.dev-reference-00ADD8?style=flat-square&logo=go&logoColor=white)](https://pkg.go.dev/github.com/sebastienrousseau/draft/engine)

Two backends ship with `draft`: **session providers** (Claude, Codex, Copilot,
Cursor and more), each driven through its own CLI in headless mode using the
user's already-authenticated session — no API token — and **Ollama**, the local
HTTP server, used offline or when no session CLI is available. Callers depend
only on `Engine`, so the pipeline is identical regardless of which one runs.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [API](#api)
- [Providers](#providers)
- [The fallback chain](#the-fallback-chain)
- [Timeouts](#timeouts)
- [Truncation](#truncation)
- [License](#license)

## Install

```sh
go get github.com/sebastienrousseau/draft@latest
```

```go
import "github.com/sebastienrousseau/draft/engine"
```

## Quick start

Implementing the seam is the whole contract:

```go
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/sebastienrousseau/draft/engine"
)

// echoEngine satisfies engine.Engine. Accept interfaces, return structs.
type echoEngine struct{ maxChars int }

func (echoEngine) Name() string { return "echo" }

func (e echoEngine) Generate(ctx context.Context, req engine.Request) (engine.Result, error) {
	if err := ctx.Err(); err != nil {
		return engine.Result{}, err // honour cancellation before doing work
	}
	text := strings.ToUpper(req.Prompt)
	if e.maxChars > 0 && len(text) > e.maxChars {
		// Truncated tells the pipeline to continue generation rather than
		// save a mid-sentence article.
		return engine.Result{Text: text[:e.maxChars], Truncated: true}, nil
	}
	return engine.Result{Text: text}, nil
}

func main() {
	var eng engine.Engine = echoEngine{maxChars: 12}

	res, err := eng.Generate(context.Background(), engine.Request{
		Kind:   engine.KindWrite, // KindExtract, KindWrite or KindEdit
		Prompt: "write something grounded",
	})
	if err != nil {
		fmt.Println("generate failed:", err)
		return
	}
	fmt.Printf("%s: %q truncated=%v\n", eng.Name(), res.Text, res.Truncated)
	// Output: echo: "WRITE SOMETH" truncated=true
}
```

Inspecting what is installed, without generating anything:

```go
package main

import (
	"fmt"

	"github.com/sebastienrousseau/draft/engine"
)

func main() {
	for _, name := range engine.ProviderNames() { // preference order
		p, ok := engine.LookupProvider(name)
		if !ok {
			continue
		}
		status := "stable"
		if p.Experimental {
			status = "experimental"
		}
		fmt.Printf("%-13s %s\n", p.Name, status)
	}

	// false skips experimental providers, matching auto mode's default.
	if p, ok := engine.FirstAvailableProvider(false); ok {
		fmt.Println("auto would pick:", p.Name)
	} else {
		fmt.Println("no session CLI installed; auto falls back to Ollama")
	}
}
```

## API

| Symbol | Signature | Purpose |
| ------ | --------- | ------- |
| `Engine` | `interface{ Name() string; Generate(context.Context, Request) (Result, error) }` | The seam every backend implements |
| `Request` | `struct{ Kind; Prompt string; Temperature float64; NumPredict int; OnChunk func(string) }` | One generation call |
| `Result` | `struct{ Text string; Truncated bool }` | Its outcome |
| `Kind` | `KindExtract`, `KindWrite`, `KindEdit` | Which stage the request belongs to, so a backend can pick a model |
| `Chain` | `func(cfg config.Config) []Engine` | The ordered list of engines to try |
| `Validate` | `func(cfg config.Config) error` | Rejects an engine name that does not exist; call once before `Chain` |
| `NewOllama` | `func(cfg config.Config) *Ollama` | Local HTTP backend |
| `NewSession` | `func(name string, cfg config.Config) (*Session, bool)` | Named session-provider backend; `false` if unknown |
| `Providers` | `[]Provider` | The registry, in auto-selection preference order |
| `ProviderNames` | `func() []string` | Every registered name, in that order |
| `LookupProvider` | `func(name string) (Provider, bool)` | Spec for one provider |
| `FirstAvailableProvider` | `func(includeExperimental bool) (Provider, bool)` | First registered provider whose CLI is on `PATH` |
| `ResolveModel` | `func(cfg config.Config, e Engine) string` | The model label an engine will use, for display |

`Request.OnChunk`, when set, receives streamed text as it arrives — that is what
drives the dashboard's live preview. `Request.NumPredict` caps output tokens for
one call; session providers manage their own generation and ignore it.

## Providers

Rows are in auto-selection preference order. Invocations were derived from each
CLI's own `--help`.

| # | Provider | Status | Headless invocation |
| - | -------- | ------ | ------------------- |
| 1 | `claude` | stable | `claude -p --output-format stream-json --include-partial-messages --verbose` (stdin) |
| 2 | `copilot` | stable | `copilot -p --allow-all-tools` |
| 3 | `codex` | stable | `codex exec` (stdin) |
| 4 | `agy` | stable | `agy -p` |
| 5 | `cursor-agent` | stable | `cursor-agent -p --output-format text --force` (stdin) |
| 6 | `amp` | experimental | `amp -x` |
| 7 | `crush` | experimental | `crush run` |
| 8 | `goose` | experimental | `goose run --no-session -t` |
| 9 | `grok` | stable | `grok --output-format plain --single` |
| 10 | `qwen` | experimental | `qwen -p` |

**Experimental** means the invocation is correct per `--help` but the output has
not been verified for a full article, so auto-selection skips it unless
`--experimental` is set.

Rows marked **(stdin)** receive the prompt on standard input rather than as a
command-line argument, so it does not appear in a process listing alongside the
source text it quotes. That flag is set only where stdin delivery was confirmed
by running the CLI: the others were either confirmed to ignore stdin, or could
not be run to check.

## The fallback chain

`Chain` resolves the ordered list of engines to try. The pipeline uses the first
that succeeds and sticks with it.

- `"ollama"` — just the local backend.
- a provider name — that session provider, then Ollama.
- `"auto"` (the default) — every installed session provider in preference
  order, then Ollama.

There is deliberately **no up-front network probe**. A flaky connectivity check
must never be what downgrades an online machine to the local model. Instead, if
a session call fails — offline, or not logged in — the pipeline advances to the
next engine in the chain and stays there for the rest of the run.

Cancellation is not a failure. A context that is cancelled or times out stops
the run where it is rather than falling over to the next backend, which would
retry work the user just abandoned.

`Chain` cannot report an unknown engine name — it returns no error, so it
degrades to Ollama. Call `Validate` first, or a typo produces a local-model
draft the user believes came from a session provider:

```go
if err := engine.Validate(cfg); err != nil {
	log.Fatal(err) // unknown engine "claud" (want auto, ollama, or one of: ...)
}
chain := engine.Chain(cfg)
```

## Timeouts

Every generation call is bounded by `config.Config.CallTimeout` (30 minutes by
default, `DRAFT_CALL_TIMEOUT=0` to disable). The ceiling is generous because
generation is legitimately slow; its job is to stop a wedged provider CLI or a
black-holed HTTP connection from hanging a run forever.

The Ollama backend uses its own `http.Client`, not `http.DefaultClient` — that
one is process-global and has no timeout at all. A blanket `Client.Timeout`
would be wrong here because it bounds the streamed body too, so the limits sit
on the dial and on the wait for response headers, with the overall call bounded
by the context.

## Truncation

`Result.Truncated` tells the pipeline to continue generation rather than save a
mid-sentence article. Ollama reports it from `done_reason`, and the stream-json
providers from the `message_delta` stop reason. Providers that return plain
text cannot report it at all, so the pipeline also treats an ending that does
not close a sentence as a truncation — otherwise continuation would be dead
code for most backends.

## License

Licensed under either of [Apache License 2.0](../LICENSE-APACHE) or
[MIT License](../LICENSE-MIT), at your option. © Sebastien Rousseau.

<p align="right"><a href="#draftengine">Back to top ↑</a></p>
