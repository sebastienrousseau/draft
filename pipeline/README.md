# draft/pipeline

Orchestrates a single drafting job end to end, UI-agnostic and engine-agnostic.

[![Go reference](https://img.shields.io/badge/go.dev-reference-00ADD8?style=flat-square&logo=go&logoColor=white)](https://pkg.go.dev/github.com/sebastienrousseau/draft/pipeline)
[![Website](https://img.shields.io/badge/draftlib.com-ff6b5a?style=flat-square)](https://draftlib.com)

Extract source text, mine quote-verified claims, write a grounded article
(continuing past length limits and retrying on rule violations), validate it,
and save the set. Progress is reported through an `Event` channel, so the same
`Runner` drives the TUI, `--print` and `--json` without knowing about any of
them.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [API](#api)
- [Phases](#phases)
- [Events](#events)
- [License](#license)

## Install

```sh
go get github.com/sebastienrousseau/draft@latest
```

```go
import "github.com/sebastienrousseau/draft/pipeline"
```

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/pipeline"
)

// demoEngine keeps the example offline and deterministic.
type demoEngine struct{}

func (demoEngine) Name() string { return "demo" }

func (demoEngine) Generate(ctx context.Context, req engine.Request) (engine.Result, error) {
	if err := ctx.Err(); err != nil {
		return engine.Result{}, err
	}
	if req.Kind == engine.KindExtract {
		return engine.Result{Text: "CLAIM: Router-S used 5x fewer FLOPs\n" +
			"SOURCE_QUOTE: \"used 5x fewer FLOPs\"\nTYPE: result\n" +
			"STRENGTH: demonstrated\n---"}, nil
	}
	body := strings.Repeat("The grounded result stands on its own and reads plainly. ", 110)
	return engine.Result{Text: "# Router-S Cuts Compute\n\n**One number tells the story.**\n\n" +
		"<aside class=\"post-lead\"><p><strong>TL;DR.</strong> Fewer FLOPs.</p></aside>\n\n" +
		"> **Executive Summary**\n>\n> - Router-S used 5x fewer FLOPs.\n\n## What it shows\n\n" +
		body + "."}, nil
}

func main() {
	dir, err := os.MkdirTemp("", "draft-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Job.Sources are absolute paths; only the CLI resolves bare names.
	src := filepath.Join(dir, "paper.txt")
	if err := os.WriteFile(src, []byte("Router-S used 5x fewer FLOPs."), 0o644); err != nil {
		log.Fatal(err)
	}

	cfg := config.Config{HomeDir: dir, DraftsDir: dir, MaxContinue: 3}
	events := make(chan pipeline.Event, 256)

	// Run is synchronous and never closes the events channel: the caller owns
	// its lifecycle, so drive it from a goroutine and close on return.
	go func() {
		defer close(events)
		pipeline.NewRunner(cfg, []engine.Engine{demoEngine{}}, events).
			Run(context.Background(), pipeline.Job{Sources: []string{src}})
	}()

	for e := range events {
		switch ev := e.(type) {
		case pipeline.PhaseEvent:
			fmt.Printf("[%s] %s\n", pipeline.PhaseNames[ev.Index], ev.Status)
		case pipeline.DoneEvent:
			fmt.Printf("✓ %d words via %s → %s\n", ev.Words, ev.Engine, ev.OutputPath)
		case pipeline.ErrEvent:
			fmt.Println("×", string(ev)) // terminal failure; the loop ends next
		}
	}
}
```

## API

| Symbol | Signature | Purpose |
| ------ | --------- | ------- |
| `Runner` | `struct` | Executes jobs against an ordered engine chain |
| `NewRunner` | `func(cfg config.Config, engines []engine.Engine, events chan<- Event) *Runner` | Construct one over `engine.Chain(cfg)` or your own slice |
| `Run` | `func(ctx context.Context, job Job)` | Execute one job, reporting progress and a terminal event |
| `Job` | `struct{ Sources []string; ReviewPath string }` | One unit of work |
| `Event` | `any` | Sum type carried on the progress channel |
| `PhaseNames` | `[5]string` | Phase labels, in execution order |
| `NumPhases` | `const` | Phase count, for sizing UI state |

A `Job` with several `Sources` produces **one merged draft** — the CLI's
`--merge`. One source per `Job` yields one draft per source. Setting
`ReviewPath` enhances that existing draft with surgical edits grounded in the
sources, instead of writing a new one (`--review`).

## Phases

| Index | `PhaseNames` | What happens |
| ----- | ------------ | ------------ |
| `PhaseResolve` | Resolve source | Locate and validate the inputs |
| `PhaseExtract` | Read and section | Text extraction in reading order, split on headings |
| `PhaseClaims` | Extract claims | Per-section mining, verbatim-quote gate, dedupe |
| `PhaseWrite` | Write article | Grounded generation, continued past length limits |
| `PhaseSave` | Validate and save | House rules, faithfulness, the three-file set |

Both write paths — generating a new draft and enhancing one with `--review` —
run **both** gates before saving: `validate.Errors` for the house rules and
`validate.Faithfulness` for grounding. `--review` applies model-supplied
find/replace edits whose allowed reasons include `factual correction`, so
checking only the style rules there would let an ungrounded number into a
finished article.

The two paths differ in what they hold the article to. A newly generated draft
must pass outright — `draft` wrote every word of it. `--review` instead
compares the article before and after the edit and fails only on violations the
**edit introduced**: it operates on a file the user already has, which may
predate a rule, exceed the length band, or read as ungrounded against a ledger
mined from whichever sources were supplied today. Pre-existing problems are
reported as warnings rather than blocking the edit.

`--review` writes the enhanced draft through a temporary file and a rename, so
an interrupted write leaves the user's original article intact rather than
truncated.

## Events

`Run` never closes the channel — the caller owns its lifecycle.

| Event | Shape | Meaning |
| ----- | ----- | ------- |
| `LogEvent` | `string` | Human-readable progress line |
| `PhaseEvent` | `{Index int; Status string}` | `"running"`, `"done"` or `"failed"` |
| `EngineEvent` | `string` | Which backend is now doing the work |
| `TokenEvent` | `string` | A chunk of the article as it streams in |
| `DoneEvent` | `{OutputPath, RawPath string; Words int; Mode, Engine string; Duration time.Duration; Timings []PhaseTiming}` | Terminal success |
| `ErrEvent` | `string` | Terminal failure |

Exactly one of `DoneEvent` or `ErrEvent` is emitted per `Run`, and it carries
the wall-clock duration plus a per-phase breakdown, so a run's cost is
measurable rather than merely felt.

### Delivery

Two delivery rules matter if you write your own consumer:

- **Structural and terminal events block until accepted.** Dropping a
  `DoneEvent` would strand you waiting for an outcome that never comes. Give
  the channel a buffer (256 is what the CLI uses) or drain it concurrently.
- **`TokenEvent`s are dropped when the buffer is full.** They are emitted from
  inside the engine's read loop, one per streamed chunk, and exist only to
  animate a preview. A renderer that cannot keep up must slow the preview,
  never the generation. Nothing is lost: the complete article is what gets
  saved regardless of which frames were dropped.

Every send also races the run's context, so a consumer that stops draining —
your UI quitting, or you abandoning the channel — cannot wedge the `Runner`
goroutine. Cancel the context you passed to `Run` and it will unwind.

## License

Licensed under either of [Apache License 2.0](../LICENSE-APACHE) or
[MIT License](../LICENSE-MIT), at your option. © Sebastien Rousseau.

<p align="right"><a href="#draftpipeline">Back to top ↑</a></p>
