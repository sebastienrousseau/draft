# draft/validate

Enforces the house article rules before a draft is saved.

[![Go reference](https://img.shields.io/badge/go.dev-reference-00ADD8?style=flat-square&logo=go&logoColor=white)](https://pkg.go.dev/github.com/sebastienrousseau/draft/validate)

Required structure, length, banned vocabulary, emoji, truncation, and
faithfulness to the verified claim ledger. A violation triggers a targeted
rewrite in the pipeline, not a shrug.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [API](#api)
- [Errors versus warnings](#errors-versus-warnings)
- [License](#license)

## Install

```sh
go get github.com/sebastienrousseau/draft@latest
```

```go
import "github.com/sebastienrousseau/draft/validate"
```

## Quick start

```go
package main

import (
	"fmt"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/validate"
)

func main() {
	draft := "# Too Short\n\nA draft that breaks most of the rules."

	// Errors returns the hard violations that must block a save.
	// An empty slice means the draft is clean.
	for _, e := range validate.Errors(draft) {
		fmt.Println("✗", e)
	}

	// Faithfulness is separate: it cross-checks the draft against the ledger
	// that grounded it. Hard errors come first, warnings are advisory.
	ledger := []claims.Record{{
		Claim:       "Router-S used 5x fewer FLOPs",
		SourceQuote: "used 5x fewer FLOPs than the dense baseline",
		Type:        "result",
		Strength:    "demonstrated",
	}}
	errs, warnings := validate.Faithfulness("Router-S used 9x fewer FLOPs.", ledger)
	fmt.Println("errors:", errs)
	fmt.Println("warnings:", warnings)
	// Output: warnings: [numbers not found in any claim: 9]

	// The building blocks are exported too.
	fmt.Println(validate.WordCount(draft))            // alphanumeric tokens
	fmt.Println(validate.ContainsEmoji("clean ✅"))    // true
	fmt.Println(validate.EndsSentence("mid-thought")) // false — looks truncated
	fmt.Println(validate.LooksLikeArticle(draft))
}
```

## API

| Symbol | Signature | Purpose |
| ------ | --------- | ------- |
| `Errors` | `func(md string) []string` | Hard rule violations that must block a save; empty means clean |
| `Faithfulness` | `func(article string, records []claims.Record) (errs, warnings []string)` | Cross-check a draft against the verified ledger |
| `WordCount` | `func(s string) int` | Alphanumeric word tokens |
| `ContainsEmoji` | `func(s string) bool` | Pictographic or symbol emoji |
| `EndsSentence` | `func(tail string) bool` | Whether trailing text closes on sentence-ending punctuation |
| `LooksLikeArticle` | `func(s string) bool` | Whether a string resembles a Markdown article body |

## Errors versus warnings

The two functions split along a clean line: `Errors` judges the draft against
itself, `Faithfulness` judges it against its sources. Neither calls the other —
the pipeline runs both.

`Errors` checks the structural markers from [`rules`](../rules) (a `# ` title
first, `## ` sections, the post-lead aside, the executive summary), unfilled
skeleton placeholders, **both** word-count bounds, emoji, and banned words and
phrases in every inflection.

Both bounds matter: the writing prompt asks for `MinWords`–`MaxWords` and
`rules` declares that band for a finished draft, so checking only the floor
told the writer one thing and held it to another. The pipeline's `--review`
path applies this check differently — it fails only on violations an edit
introduced, so an article that was already over length stays reviewable.

`Faithfulness` returns two slices, and the distinction is deliberate:

- **Errors** block a save — a metric term the article names that no verified
  claim supports (a possible silent unit conversion), an ending that does not
  close on sentence punctuation (truncation), and near-duplicate paragraphs.
- **Warnings** are advisory — a number in the article that appears in no claim,
  or a hedged claim restated assertively — and are surfaced without failing the
  run.

Both read their vocabulary from [`rules`](../rules), so what the writer was
asked for and what the validator enforces cannot drift apart.

`Errors` is benchmarked (`validate/bench_test.go`); a full house-rule pass over
a finished draft runs in well under a millisecond, so validation is never the
reason a run is slow.

## License

Licensed under either of [Apache License 2.0](../LICENSE-APACHE) or
[MIT License](../LICENSE-MIT), at your option. © Sebastien Rousseau.

<p align="right"><a href="#draftvalidate">Back to top ↑</a></p>
