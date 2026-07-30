# draft/claims

Extracts, verifies, de-duplicates and renders the verified claim ledger that
grounds every draft.

[![Go reference](https://img.shields.io/badge/go.dev-reference-00ADD8?style=flat-square&logo=go&logoColor=white)](https://pkg.go.dev/github.com/sebastienrousseau/draft/claims)
[![Website](https://img.shields.io/badge/draftlib.com-ff6b5a?style=flat-square)](https://draftlib.com)

A claim survives only if its `SOURCE_QUOTE` is an exact substring of the section
it was drawn from **and** every number in the claim also appears in that quote.
This is the single most important defence against a model inventing plausible
facts.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [API](#api)
- [The verification gate](#the-verification-gate)
- [License](#license)

## Install

```sh
go get github.com/sebastienrousseau/draft@latest
```

```go
import "github.com/sebastienrousseau/draft/claims"
```

## Quick start

```go
package main

import (
	"fmt"

	"github.com/sebastienrousseau/draft/claims"
)

func main() {
	source := "Router-S used 5x fewer FLOPs than the dense baseline on the same corpus."

	// What a model returns from prompt.Claim. The second block is invented:
	// its quote does not occur in the source, so Parse drops it.
	extraction := `CLAIM: Router-S used 5x fewer FLOPs
SOURCE_QUOTE: "used 5x fewer FLOPs than the dense baseline"
TYPE: result
STRENGTH: demonstrated
---
CLAIM: Router-S halved training cost
SOURCE_QUOTE: "training cost fell by half"
TYPE: result
STRENGTH: demonstrated
---`

	records, dropped := claims.Parse(extraction, source)
	fmt.Printf("kept %d, dropped %d\n", len(records), dropped)
	// Output: kept 1, dropped 1

	// Verify explains a rejection, which is what the ledger reports.
	ok, why := claims.Verify(claims.Record{
		Claim:       "Router-S halved training cost",
		SourceQuote: "training cost fell by half",
		Type:        "result",
		Strength:    "demonstrated",
	}, source)
	fmt.Println(ok, why)

	// The compact ledger is what the writing prompt embeds; RenderLedger is the
	// human-readable form saved beside a draft with --keep-artifacts.
	fmt.Println(claims.RenderPromptLedger(records, 45, 14000) != "")
}
```

## API

| Symbol | Signature | Purpose |
| ------ | --------- | ------- |
| `Record` | `struct{ Claim, SourceQuote, Type, Strength string }` | One verified fact plus the verbatim span supporting it |
| `Parse` | `func(text, source string) (records []Record, dropped int)` | Reads one section's extraction output, keeping only records whose quotes verify |
| `Verify` | `func(rec Record, source string) (bool, string)` | Reports whether a record is trustworthy and, when not, why |
| `Dedupe` | `func(records []Record) []Record` | Removes records whose normalised claim text was already seen |
| `RenderLedger` | `func(records []Record, dropped int) string` | Full, human-readable ledger |
| `RenderPromptLedger` | `func(records []Record, maxClaims, maxChars int) string` | Compact ledger for the writing model, capped by count and characters |
| `Numbers` | `func(s string) map[string]bool` | Distinct numeric tokens, thousands separators stripped so `1,000` and `1000` compare equal |

## The verification gate

`Verify` applies nine checks in order, returning the first failure as its
reason string:

| # | Check | Rejection reason |
| - | ----- | ---------------- |
| 1 | `Claim` and `SourceQuote` both non-empty | `missing claim or quote` |
| 2 | quote is at least `rules.MinQuoteChars` (12) runes | `quote too short` |
| 3 | quote is valid UTF-8 | `quote is not valid UTF-8` |
| 4 | quote holds no U+FFFD replacement character | `quote contains a replacement character` |
| 5 | quote occurs in the source | `quote not found in source` |
| 6 | quote does not end mid-clause (`and`, `the`, a comma, …) | `quote is a truncated fragment` |
| 7 | `Type`, if set, is in `rules.ClaimTypes` | `invalid TYPE '…'` |
| 8 | `Strength`, if set, is in `rules.ClaimStrengths` | `invalid STRENGTH '…'` |
| 9 | every numeric token in `Claim` appears in the quote | `claim numbers absent from quote: …` |

Three details matter. Containment (check 5) is tested against a **normalised**
form of both strings — lowercased, whitespace collapsed, smart quotes folded to
ASCII — so a quote that differs from the source only in line wrapping or curly
apostrophes still verifies. `Type`/`Strength` are checked only when non-empty;
a block that omits them is accepted.

Checks 3 and 4 exist because that normalisation is lossy. `strings.ToLower`
maps every invalid UTF-8 byte to U+FFFD, so two *different* invalid byte
sequences normalise to the same string — and a fabricated quote could match a
source it does not occur in, defeating the one invariant the ledger rests on.
`FuzzParse` found it. Since normalisation can only ever introduce U+FFFD and
never other characters, a quote that is valid UTF-8 and free of U+FFFD cannot
be matched against mangled bytes. Legitimate accented and CJK quotes are
unaffected.

`Parse` does not de-duplicate. Call `Dedupe` yourself when merging records
across sections, as the pipeline does.

Dropped records are counted, and that count is surfaced in the ledger, so a
thin source visibly yields a thin ledger rather than a padded one.

`Parse` is fuzzed (`claims/fuzz_test.go`) against the invariant that a surviving
record must quote its source verbatim.

## License

Licensed under either of [Apache License 2.0](../LICENSE-APACHE) or
[MIT License](../LICENSE-MIT), at your option. © Sebastien Rousseau.

<p align="right"><a href="#draftclaims">Back to top ↑</a></p>
