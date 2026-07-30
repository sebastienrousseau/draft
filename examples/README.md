# Examples

Runnable programs demonstrating each part of [`draft`](../README.md). None
require a network, an API token, or a running model — they use in-process data,
a deterministic demo engine, or a read-only `PATH` probe — so they double as
living documentation and smoke tests.

## Contents

- [Running them](#running-them)
- [The examples](#the-examples)
- [What they demonstrate](#what-they-demonstrate)
- [License](#license)

## Running them

```sh
git clone https://github.com/sebastienrousseau/draft
cd draft
go run ./examples/dashboard
```

Start with `dashboard` to see the interface itself. Every example is a
`package main` you can read top to bottom in a couple of minutes.

## The examples

| Example | Run | What it shows |
| ------- | --- | ------------- |
| [`dashboard`](dashboard/main.go) | `go run ./examples/dashboard` | The real full-screen TUI driven by an in-process engine — queue, phases, live preview and focus timer, all animating; resize to watch the layout adapt |
| [`providers`](providers/main.go) | `go run ./examples/providers` | Session providers in auto-selection order, install status, default models |
| [`grounding`](grounding/main.go) | `go run ./examples/grounding` | Claim verification against a source, ledger rendering, grounded prompt, house-rule validation |
| [`pipeline`](pipeline/main.go) | `go run ./examples/pipeline` | The five-phase pipeline end to end, merged multi-source drafting, streamed events, day-folder output |
| [`review`](review/main.go) | `go run ./examples/review` | Surgical-edit enhancement: body-only prompting, frontmatter re-attachment, set resync |
| [`frontmatter`](frontmatter/main.go) | `go run ./examples/frontmatter` | Metadata extraction, custom `Site` identity, Split/Combine round trip, the three regeneration rules |

## What they demonstrate

Each example is built around one seam of the library, so the package it
exercises is the package to read next:

| Example | Packages exercised |
| ------- | ------------------ |
| `dashboard` | `config`, `engine`, `pipeline`, `internal/tui` |
| `providers` | `engine` (`Providers`, `ProviderNames`, `LookupProvider`) |
| `grounding` | `claims`, `prompt`, `rules`, `validate` |
| `pipeline` | `config`, `engine`, `pipeline` |
| `review` | `config`, `engine`, `frontmatter`, `pipeline` |
| `frontmatter` | `frontmatter` |

The pattern every example shares is the `engine.Engine` seam — a two-method
interface a demo backend can satisfy in a dozen lines:

```go
package main

import (
	"context"
	"fmt"

	"github.com/sebastienrousseau/draft/engine"
)

// demoEngine returns fixed text, so an example is deterministic and offline.
type demoEngine struct{}

func (demoEngine) Name() string { return "demo" }

func (demoEngine) Generate(ctx context.Context, req engine.Request) (engine.Result, error) {
	if err := ctx.Err(); err != nil {
		return engine.Result{}, err // cancellation propagates before any work
	}
	if req.Kind == engine.KindExtract {
		return engine.Result{Text: "CLAIM: ...\nSOURCE_QUOTE: \"...\"\n---"}, nil
	}
	return engine.Result{Text: "# A Grounded Article\n\n..."}, nil
}

func main() {
	var eng engine.Engine = demoEngine{}
	res, err := eng.Generate(context.Background(), engine.Request{Kind: engine.KindWrite})
	if err != nil {
		fmt.Println("generate failed:", err)
		return
	}
	fmt.Println(eng.Name(), "produced", len(res.Text), "bytes")
}
```

For the real command — driving a session provider (online) or Ollama (offline)
over your PDFs — see the top-level [README](../README.md). For the package API,
see the [Go reference](https://pkg.go.dev/github.com/sebastienrousseau/draft).

## License

Licensed under either of [Apache License 2.0](../LICENSE-APACHE) or
[MIT License](../LICENSE-MIT), at your option. © Sebastien Rousseau.

<p align="right"><a href="#examples">Back to top ↑</a></p>
