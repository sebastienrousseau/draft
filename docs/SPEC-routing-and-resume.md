<!-- markdownlint-disable MD013 -->
# Spec: engine routing, resume, and run planning

Status: **proposed** · Target: `0.0.29` · Author: drafted for review

This specifies five changes aimed at the only thing that actually costs time in
a `draft` run — model calls — plus the one bug found while designing them.

## Contents

- [Why these five](#why-these-five)
- [1. Per-`Kind` engine routing](#1-per-kind-engine-routing)
- [2. Ledger resume](#2-ledger-resume)
- [3. `--dry-run`](#3---dry-run)
- [4. Queue-sticky fallback](#4-queue-sticky-fallback)
- [5. Extraction ETA](#5-extraction-eta)
- [Not doing yet: extraction batching](#not-doing-yet-extraction-batching)
- [Rollout](#rollout)

---

## Why these five

The repository's own measurements set the frame. The deterministic Go path —
extraction, sectioning, claim verification, house-rule validation — runs in
**~110 ms**. Claim extraction against a local model on a 12-section paper runs
in **~645–825 s**. Model calls are over 99% of wall clock.

That has two consequences:

1. **Go-side optimisation is finished.** The one real hotspot (the TUI preview
   handler, quadratic in article length) is fixed. Anything further is noise
   against a 10-minute run.
2. **Every remaining lever is about model calls**: making fewer of them, routing
   them to a cheaper backend, or not repeating ones already paid for.

Ranked by value ÷ effort:

| # | Change | Lever | Effort |
| - | ------ | ----- | ------ |
| 1 | Per-`Kind` engine routing | ~13 session calls per paper → 1 | Medium |
| 2 | Ledger resume | A failed run costs seconds, not 10 minutes | Medium |
| 3 | `--dry-run` | Stop committing to a long run blind | Small |
| 4 | Queue-sticky fallback | Removes N× repeated dead-provider retries | Small |
| 5 | Extraction ETA | Tells you if this is 2 or 20 minutes | Small |

---

## 1. Per-`Kind` engine routing

### Problem

`engine.Chain(cfg)` returns **one chain for every request kind**. But the
workload is lopsided:

| Kind | Calls per paper | Nature | Wants |
| ---- | --------------- | ------ | ----- |
| `KindExtract` | one per section (~12) | mechanical: find quotes, copy them verbatim | cheap, local, parallel |
| `KindWrite` | 1 (+ retries/continuations) | the quality-critical call | the best writer available |
| `KindEdit` | 1, only on `--review` | surgical find/replace | either |

Today you choose one backend for all of it. Choosing `claude` burns ~13 session
calls per paper on work a 4B local model does fine. Choosing `ollama` gets local
extraction but also a 4B writer.

The seam for fixing this already exists: `engine.Request.Kind` is threaded
through every call, and `Ollama.modelFor` already switches *models* on it. This
extends that from models to *engines*.

### Design

Three new optional config fields, each falling back to `Engine` when unset:

```go
type Config struct {
    Engine        string // existing: the default for every kind
    ExtractEngine string // DRAFT_EXTRACT_ENGINE
    WriteEngine   string // DRAFT_WRITE_ENGINE
    EditEngine    string // DRAFT_EDIT_ENGINE
    // ...
}
```

The intended configuration becomes:

```sh
DRAFT_EXTRACT_ENGINE=ollama   # ~12 calls, local, free, works on a plane
DRAFT_WRITE_ENGINE=claude     # 1 call, best available writer
```

Each kind keeps its **own independent fallback chain and cursor**. Extraction
failing over to Ollama must not drag writing down with it, and vice versa.

### API

```go
// engine

// ChainFor resolves the ordered chain for one request kind, honouring the
// per-kind override and falling back to cfg.Engine.
func ChainFor(cfg config.Config, kind Kind) []Engine

// Chain is ChainFor(cfg, KindWrite). Retained so existing callers and the
// published examples keep working.
func Chain(cfg config.Config) []Engine

// Validate now checks Engine, ExtractEngine, WriteEngine and EditEngine,
// naming whichever is wrong.
func Validate(cfg config.Config) error
```

```go
// pipeline

// NewRunner is unchanged: one chain, used for every kind. Every existing
// caller, test and documented example keeps compiling and behaving identically.
func NewRunner(cfg config.Config, engines []engine.Engine, events chan<- Event) *Runner

// NewRoutedRunner resolves a separate chain per kind from cfg.
func NewRoutedRunner(cfg config.Config, events chan<- Event) *Runner
```

Internally `Runner.engines []engine.Engine` + `cur int` becomes:

```go
type chainState struct {
    engines []engine.Engine
    cur     int
}

type Runner struct {
    chains map[engine.Kind]*chainState
    // ...
}

func (r *Runner) chainFor(k engine.Kind) *chainState
```

`NewRunner` populates all three kinds with the *same* `*chainState` pointer, so
a uniform runner keeps one shared cursor — preserving today's behaviour exactly,
including cross-kind stickiness. `NewRoutedRunner` gives each kind its own.

### Files

| File | Change |
| ---- | ------ |
| `config/config.go` | 3 fields; read `DRAFT_{EXTRACT,WRITE,EDIT}_ENGINE`; no clamping needed (validated by `engine`) |
| `engine/engine.go` | extract `chainForName`; add `ChainFor`; `Chain` delegates; `Validate` covers all four names |
| `pipeline/pipeline.go` | `chainState`, `chains` map, `chainFor`, `generate` uses the per-kind cursor; `Run` no longer resets `engineName` from `engines[0]` |
| `cmd/draft/main.go` | use `NewRoutedRunner`; `--extract-engine` / `--write-engine` flags |
| `internal/tui/tui.go` | same |
| docs | `engine/README.md`, `config/README.md`, root README config table |

### Semantics to pin with tests

- An unset override inherits `Engine`.
- A per-kind override wins over `Engine`.
- Extraction failing over does **not** advance the write cursor.
- `Validate` names *which* setting is invalid (`DRAFT_WRITE_ENGINE`, not just "engine").
- `NewRunner` with a flat slice behaves exactly as today (shared cursor).
- `DoneEvent.Engine` reports the **writing** engine, since that is the one that
  produced the article.

### Risks

- **`DoneEvent.Engine` becomes ambiguous** under routing. Resolved above: it
  reports the writer. `--json` consumers keying on it see no format change.
- **Two backends warm at once** costs memory if both are Ollama models. Already
  mitigated: all three Ollama stages default to the same `gemma3:4b`.

---

## 2. Ledger resume

### Problem

Extraction is 80–95% of a run. When the write phase fails after `WriteRetries`,
`run` returns an error and every extracted claim is thrown away — even though
`cleanupArtifacts` only deletes the ledger on **success**, so the verified
ledger is *already sitting on disk*. Nothing reads it back.

Re-running means re-paying ~10 minutes for work that completed correctly.

### Bug found while specifying this

```go
ledgerPath := filepath.Join(outputDir, time.Now().Format("2006-01-02")+"-verified-claim-ledger.md")
```

The name is **per-day, not per-source**. Draft two papers on the same day and
the second silently overwrites the first's ledger. With `--keep-artifacts` you
keep only the last one. This must be fixed before resume can key on it.

**Fix:** `<date>-<slug of the first source stem>-verified-claim-ledger.md`,
reusing the existing `slugify`. Also add a `TestLedgersDoNotCollideAcrossJobs`.

### Design

```
draft --resume paper.pdf
```

1. Resolve and section the sources as normal (~110 ms — cheap enough to always
   redo, and it is what makes re-verification possible).
2. Look for the ledger for these sources in today's folder.
3. If found, parse it and **re-verify every record against the freshly
   sectioned text**. A record whose quote no longer appears — because the
   source changed — is dropped and reported.
4. Skip `PhaseClaims` entirely; proceed to write.

Re-verification is the important part: a resumed ledger is not trusted because
it was written by us, it is trusted because it still passes the same gate.
Resume therefore cannot weaken grounding.

### API

`claims.RenderLedger` emits a header before the records:

```text
# Verified Claim Ledger

Verified records: 3
Dropped records with unverifiable SOURCE_QUOTE: 1

CLAIM: ...
```

Feeding that straight to `claims.Parse` works, but the header forms a block
with no `CLAIM:` and inflates the dropped count by one. So:

```go
// ParseLedger reads back a ledger previously written by RenderLedger,
// re-verifying every record against source. The rendered header is skipped, so
// the dropped count reflects records that genuinely no longer verify.
func ParseLedger(ledger, source string) (records []Record, dropped int)
```

### Files

| File | Change |
| ---- | ------ |
| `claims/claims.go` | `ParseLedger` (strip everything before the first `CLAIM:`, delegate to `Parse`) |
| `pipeline/pipeline.go` | per-source ledger name; `resumeLedger()`; `run` skips `PhaseClaims` on a hit |
| `config/config.go` | `Resume bool` |
| `cmd/draft/main.go` | `--resume` flag + help entry |

### Semantics to pin with tests

- Round trip: `RenderLedger` → `ParseLedger` returns the same records, `dropped == 0`.
- A record whose source changed is dropped on resume and reported.
- `--resume` with no ledger present falls back to a normal run (not an error).
- Two jobs on one day produce two distinct ledger files.
- Resume performs **zero** `KindExtract` calls (assert on a counting fake engine).

### Risks

- **Stale ledger against edited sources.** Mitigated by mandatory
  re-verification; a wholly changed source resumes to an empty ledger, which
  then fails the write phase honestly rather than producing an ungrounded draft.
- **`--resume` + `--merge`** must key on the same source set. Key on the first
  source's slug plus the source count.

---

## 3. `--dry-run`

### Problem

You commit to a 10-minute run blind. There is no way to ask: which engine will
this use, how many sections is this paper, how many model calls will that be?

### Design

`--dry-run` runs resolve + read + section (~110 ms), prints a plan, exits 0
without a single model call:

```text
draft --dry-run 2603.23420.pdf

Plan
  Sources          1  (2603.23420.pdf, 412 KB)
  Sections         12
  Engines          extract: ollama · write: claude · edit: claude
  Model calls      ~13  (12 extract + 1 write, plus up to 2 retries)
  Output           ~/Drop/Drafts/2026-07-30/
  Resumable        no ledger found
```

### API

```go
// pipeline

// DryRunReport is what a run would do, without doing it.
type DryRunReport struct {
    Sources      []string
    SectionCount int
    Engines      map[engine.Kind]string
    EstCalls     int
    OutputDir    string
    LedgerFound  bool
}

func (r *Runner) DryRun(ctx context.Context, job Job) (DryRunReport, error)
```

Reuses `sections()` verbatim, so it exercises the same resolve and extraction
path a real run would — a dry run that succeeds is real evidence the sources
are readable.

---

## 4. Queue-sticky fallback

### Problem

`NewRunner` is constructed **per job** in both `cmd/draft/headless.go` and
`internal/tui/tui.go`, so `cur` resets to 0 every time. Fallback is sticky
*within* a run but not *across a queue*. Point `draft` at 20 papers while
offline and every dead provider is retried 20 times, re-emitting the same
`falling back to …` warnings each round.

### Design

Construct **one** `Runner` and call `Run` per job. `Run` already re-binds
`done`, `started` and `timings` per job; the chain cursor simply stops being
reset. One line changes in each of three call sites, plus removing the
`engineName = engines[0].Name()` reset in `Run` (it must read the *current*
write-chain engine instead).

### Semantics to pin with tests

- Two jobs, primary engine dead: the primary is attempted **once**, not twice.
- A per-job `DoneEvent` still reports the engine that served that job.

---

## 5. Extraction ETA

`DoneEvent` now carries per-phase timings, and extraction is a countable N. After
section 1 completes, per-section time is known:

```text
claim section 3/12 · ~4m20s remaining
```

Emit as part of the existing `LogEvent` line rather than a new event type, and
show it in the dashboard status line. Estimate from a rolling mean of completed
sections, not just the first (the first is unrepresentative — it settles the
engine chain and warms the model).

---

## Not doing yet: extraction batching

Sending 3–4 sections per `KindExtract` call would cut the dominant cost 3–4×.
It is **not** in this spec, because it fights a deliberate design decision:

```go
// MaxSectionChars bounds a single extraction section so a small local model is
// never asked to hold more than it can attend to at once.
const MaxSectionChars = 4500
```

Quote verification stays sound when batched — verify against the concatenated
batch, and a quote must still appear in real source text — so the risk is not
correctness but **claim quality** on a 4B model given 18,000 characters.

**Proposed experiment before any code**, on a fixed corpus of ~10 papers:

| Arm | Sections per call |
| --- | ----------------- |
| A (control) | 1 |
| B | 2 |
| C | 4 |

Measure: wall clock, claims verified, claims dropped, and — the one that
matters — claims *found by A but missed by B/C*. Batching is worth shipping
only if recall holds within a few percent. Ship behind
`DRAFT_EXTRACT_BATCH=n`, default 1, if it does.

---

## Rollout

Five independent commits, each green on its own:

1. `fix(pipeline)`: per-source ledger filename *(bug fix, ships regardless)*
2. `feat(engine)`: per-`Kind` routing
3. `feat(pipeline)`: ledger resume
4. `feat(cmd)`: `--dry-run`
5. `fix(cmd)`: queue-sticky fallback + ETA

Every one is additive. `NewRunner`, `engine.Chain`, all existing env vars and
every documented example keep working unchanged, so there is no migration step
and nothing to deprecate.

Gate for each: `make check`, `golangci-lint run`, `go test -race ./...`,
coverage ≥ 95%, `make fuzz`, and the nine package-README programs still
compiling and running.
