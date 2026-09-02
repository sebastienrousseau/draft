# draft/rules

The shared editorial constants the prompt builder and the validator both depend
on.

[![Go reference](https://img.shields.io/badge/go.dev-reference-00ADD8?style=flat-square&logo=go&logoColor=white)](https://pkg.go.dev/github.com/sebastienrousseau/draft/rules)
[![Website](https://img.shields.io/badge/draftlib.com-ff6b5a?style=flat-square)](https://draftlib.com)

Word limits, banned vocabulary, metric vocabulary and the accepted claim
taxonomy live here and nowhere else. Keeping them in one place guarantees the
writer is told exactly what the validator enforces, and that the two cannot
silently drift apart.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [API](#api)
- [Why one package](#why-one-package)
- [License](#license)

## Install

```sh
go get github.com/sebastienrousseau/draft@latest
```

```go
import "github.com/sebastienrousseau/draft/rules"
```

## Quick start

```go
package main

import (
	"fmt"

	"github.com/sebastienrousseau/draft/rules"
)

func main() {
	fmt.Printf("a draft must be %d-%d words\n", rules.MinWords, rules.MaxWords)

	// Banned words are matched on every inflection, not just the base form.
	// WordForm pairs the surface form with the inflection kind that produced
	// it ("base", "s", "ed" or "ing"), so a replacement can be inflected to match.
	for _, f := range rules.WordForms("leverage") {
		fmt.Printf("  %s (%s)\n", f.Form, f.Kind)
	}

	// Each banned term maps to a neutral replacement, so one stray cliché is
	// repaired in place instead of costing a full regeneration.
	fmt.Println("leverage →", rules.StyleReplacements["leverage"])

	// The claim taxonomy the verifier accepts.
	fmt.Println("valid TYPE 'result'?    ", rules.ClaimTypes["result"])
	fmt.Println("valid STRENGTH 'hedged'?", rules.ClaimStrengths["hedged"])

	// Structural markers the writer must emit and the validator checks for.
	fmt.Println(rules.H1Prefix, rules.H2Prefix, rules.ExecSummaryMarker)
}
```

## API

| Symbol                                                             | Kind                             | Purpose                                                                                                                                                   |
| ------------------------------------------------------------------ | -------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `MinWords`, `MaxWords`                                             | `const`                          | Word-count bounds for a finished body-only draft (500–3000). Both are enforced by `validate.Errors`, and the pipeline sizes generation against `MaxWords` |
| `MinQuoteChars`                                                    | `const`                          | Shortest verbatim source span a claim may cite (12)                                                                                                       |
| `PostLeadAsideMarker`, `ExecSummaryMarker`, `H1Prefix`, `H2Prefix` | `const`                          | Structural markers the writer emits and the validator checks                                                                                              |
| `BannedWords`, `BannedPhrases`                                     | `var []string`                   | Single tokens and multi-word clichés the house style forbids                                                                                              |
| `StyleReplacements`                                                | `var map[string]string`          | Every banned term → a neutral equivalent                                                                                                                  |
| `AssertiveVerbs`                                                   | `var []string`                   | Verbs that state a result as settled fact                                                                                                                 |
| `MetricTerms`                                                      | `var []string`                   | Evaluation metrics that must not appear ungrounded                                                                                                        |
| `WriterStopwords`                                                  | `var map[string]bool`            | Common words ignored when measuring token overlap                                                                                                         |
| `ClaimTypes`, `ClaimStrengths`, `HedgeStrengths`                   | `var map[string]bool`            | The accepted claim taxonomy                                                                                                                               |
| `WordForms`                                                        | `func(w string) []WordForm`      | A banned word plus its common inflections, as `{Form, Kind}` pairs                                                                                        |
| `InflectLike`                                                      | `func(word, kind string) string` | Inflect a word using regular English spelling rules                                                                                                       |
| `MetricForms`                                                      | `func(term string) []string`     | Every surface form equivalent to a metric term                                                                                                            |

`ClaimTypes` accepts `metric`, `mechanism`, `definition`, `method`, `result` and
`limitation`. `ClaimStrengths` accepts `demonstrated`, `hedged` and
`speculation-or-future-work`; the latter two are also in `HedgeStrengths`.

## Why one package

`prompt` reads these constants to tell the model what is expected;
`validate` reads the same constants to enforce it; `claims` reads
`MinQuoteChars`, `ClaimTypes` and `ClaimStrengths` to gate a record. Three
consumers, one definition.

`TestSkeletonMatchesStructureMarkers` asserts that the writing prompt's output
skeleton embeds each structural marker, so a change here that the prompt does
not follow fails the build rather than the draft.

This package holds constants only. It imports nothing from the rest of the
module, which is what lets everything else import it.

## License

Licensed under either of [Apache License 2.0](../LICENSE-APACHE) or
[MIT License](../LICENSE-MIT), at your option. © Sebastien Rousseau.

<p align="right"><a href="#draftrules">Back to top ↑</a></p>
