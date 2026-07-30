# draft/prompt

Builds the grounded prompts sent to a generation backend.

[![Go reference](https://img.shields.io/badge/go.dev-reference-00ADD8?style=flat-square&logo=go&logoColor=white)](https://pkg.go.dev/github.com/sebastienrousseau/draft/prompt)

Three prompts, one per stage: per-section claim extraction, article writing
(with an embedded output skeleton and the compact claim ledger), and surgical
review. They are backend-agnostic — the same text goes to Claude or to Ollama —
so quality does not drift between engines.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [API](#api)
- [Grounding and prompt injection](#grounding-and-prompt-injection)
- [License](#license)

## Install

```sh
go get github.com/sebastienrousseau/draft@latest
```

```go
import "github.com/sebastienrousseau/draft/prompt"
```

## Quick start

```go
package main

import (
	"fmt"
	"strings"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/prompt"
	"github.com/sebastienrousseau/draft/rules"
)

func main() {
	source := "Router-S used 5x fewer FLOPs than the dense baseline on the same corpus."

	// 1. Ask a backend to mine this section for claims.
	extractPrompt := prompt.Claim(source)
	fmt.Println("extract prompt:", len(extractPrompt), "chars")

	// 2. Verify what came back, then render the ledger the writer may use.
	records, _ := claims.Parse(`CLAIM: Router-S used 5x fewer FLOPs
SOURCE_QUOTE: "used 5x fewer FLOPs than the dense baseline"
TYPE: result
STRENGTH: demonstrated
---`, source)
	ledger := claims.RenderPromptLedger(records, 45, 14000)

	// 3. Build the writing prompt. templates may be empty; the ledger is
	//    substituted for LedgerPlaceholder inside the template.
	writing := prompt.Writing("", ledger, rules.MinWords, rules.MaxWords)
	fmt.Println("ledger embedded:", strings.Contains(writing, "Router-S"))
	fmt.Println("placeholder consumed:", !strings.Contains(writing, prompt.LedgerPlaceholder))

	// 4. If the backend stops on a length limit, ask it to finish.
	fmt.Println("continue prompt:", len(prompt.ContinueWriting("...half an article")) > 0)
}
```

## API

| Symbol | Signature | Purpose |
| ------ | --------- | ------- |
| `Claim` | `func(source string) string` | Extraction prompt for a single source section |
| `Writing` | `func(templates, ledger string, minWords, maxWords int) string` | Article prompt; `templates` may be empty, `ledger` is required |
| `ContinueWriting` | `func(partial string) string` | Nudges a backend that stopped on a length limit to finish cleanly |
| `Review` | `func(research, draft, ledger string) string` | Surgical find/replace edit prompt |
| `EffectiveStyle` | `func(templates string) string` | The style-calibration text the writing prompt will actually use |
| `LedgerPlaceholder` | `const` `"{{VERIFIED_CLAIM_LEDGER}}"` | Substituted with the compact ledger |
| `MaxReviewSourceChars`, `MaxDraftChars` | `const` | Bounds on the review prompt's inputs |

## Grounding and prompt injection

The claim ledger is the only permitted source of facts in the writing prompt.
The writer arranges facts; it does not source them.

Source text and style templates are embedded as **quoted, untrusted evidence**,
and the prompt instructs the model to ignore any instructions found inside them.
Treat that as defence in depth rather than a guarantee: a session provider runs
with its own tool permissions, so prefer Ollama for material you do not trust.

Word bounds come from [`rules`](../rules), the same package the validator reads,
so the writer is told exactly what will be enforced.

## License

Licensed under either of [Apache License 2.0](../LICENSE-APACHE) or
[MIT License](../LICENSE-MIT), at your option. © Sebastien Rousseau.

<p align="right"><a href="#draftprompt">Back to top ↑</a></p>
