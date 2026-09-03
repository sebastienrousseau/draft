# Architecture

How `draft` fits together, for people changing it. The README explains what it
does; this explains why the pieces are shaped the way they are, and which
invariants a change must not break.

## Contents

- [The one-paragraph version](#the-one-paragraph-version)
- [Package graph](#package-graph)
- [The pipeline, phase by phase](#the-pipeline-phase-by-phase)
- [Invariants](#invariants)
- [The two seams](#the-two-seams)
- [Where a change usually goes](#where-a-change-usually-goes)

## The one-paragraph version

A source document is turned into plain text, split into bounded sections, and
each section is sent to a model that returns candidate claims. A claim survives
only if its quote is a verbatim substring of the section it came from. The
surviving claims — and nothing else — are the facts a second model call is
allowed to use when it writes the article. The result is checked against house
rules and against the ledger before it is saved.

The order matters: **verification happens before writing, not after.** A model
that never sees an unverified fact cannot repeat one.

## Package graph

```text
cmd/draft ──────────► pipeline ──────► engine ──► (agent CLI | Ollama)
   │                     │  │  │
   │                     │  │  └────► prompt ───► rules
   │                     │  └───────► claims ──► rules
   │                     ├──────────► validate ► claims, rules
   │                     ├──────────► frontmatter
   │                     └──────────► internal/{pdf,extractcache,mdspan,runes,atomicfile}
   └───────────────────► internal/tui, internal/brand, config
```

| Package                 | Responsibility                                                              | Depends on            |
| ----------------------- | --------------------------------------------------------------------------- | --------------------- |
| `cmd/draft`             | Flag parsing, job planning, headless/JSON/TUI dispatch, `--doctor`          | everything            |
| `config`                | Resolve flags > environment > defaults; clamp and report                    | —                     |
| `engine`                | One interface over agent CLIs and Ollama; provider registry; failover chain | `config`              |
| `prompt`                | Build the extraction, writing, continuation and review prompts              | `rules`               |
| `claims`                | Parse, **verify**, de-duplicate and render the claim ledger                 | `rules`               |
| `validate`              | House rules and faithfulness checks on a finished draft                     | `claims`, `rules`     |
| `rules`                 | The house vocabulary, length band, claim types, metric groups               | —                     |
| `frontmatter`           | Split, generate and recombine YAML frontmatter                              | —                     |
| `pipeline`              | Orchestrate a job; emit progress events                                     | all of the above      |
| `internal/pdf`          | Extract text and split it into bounded sections                             | —                     |
| `internal/extractcache` | Content-addressed reuse of extraction output                                | `internal/atomicfile` |
| `internal/mdspan`       | Locate code and quoted spans that must not be rewritten                     | —                     |
| `internal/runes`        | UTF-8-safe cut points                                                       | —                     |
| `internal/atomicfile`   | Write-and-rename, so a crash cannot truncate a draft                        | —                     |
| `internal/tui`          | The Bubble Tea dashboard                                                    | `pipeline`, `engine`  |

`rules` has no dependencies on purpose: it is the vocabulary both the prompt
and the validator read, so the model is asked for the same thing it is judged
against. When they disagree, a draft fails a rule it was never told about.

## The pipeline, phase by phase

`pipeline.Runner.run` walks five phases, reporting each on the event channel.

| # | Phase             | What happens                                                                       | Cost                           |
| - | ----------------- | ---------------------------------------------------------------------------------- | ------------------------------ |
| 0 | Resolve           | Sources to absolute paths                                                          | negligible                     |
| 1 | Read and section  | `pdftotext`, normalise, split at ≤4,500 chars on paragraph or sentence boundaries  | ~0.25 s                        |
| 2 | Extract claims    | One model call per section; each result parsed and **verified**; cached by content | **80–95% of wall clock**       |
| 3 | Write             | One model call with the compact ledger; continue past length limits                | the single most expensive call |
| 4 | Validate and save | House rules, faithfulness, deterministic repair, retry, day-folder set             | ~10 ms                         |

Two details that are easy to miss:

- **The write budget scales with the ledger.** `writeBudget(len(records))` sizes
  the target length to the amount of grounded material, because asking for 3,000
  words from six claims produces padding, and padding is what trips the
  faithfulness checks into an expensive retry.
- **Repair before regeneration.** A rule violation used to cost a full rewrite.
  Banned vocabulary and near-duplicate paragraphs are now fixed deterministically
  first; only grounding failures escalate to another model call.

## Invariants

Break one of these and the product stops being what it claims to be. Each is
pinned by a test.

1. **A claim's quote occurs verbatim in its source section.** After
   normalisation — case, whitespace, smart quotes — and never through a
   replacement character. `claims.Verify`, mutation-tested at 100% efficacy.
2. **Every number in a claim also appears in its quote.** Otherwise a model can
   attach a real quote to an invented figure.
3. **A cached or resumed ledger is re-verified, never trusted.** Reuse can only
   ever produce *fewer* claims, never an ungrounded one.
4. **Nothing rewrites text inside a quotation.** The house-vocabulary repair
   runs only outside code and quoted spans (`internal/mdspan`); rewriting there
   would attribute words to a source that never wrote them.
5. **Untrusted text is fenced and never granted tools.** Source documents reach
   an agent in a nonce-delimited block, in an empty working directory, with no
   tool-granting flags.
6. **A draft is written as a set or not at all.** Body, frontmatter and final
   document share a stem claimed with `O_EXCL`, so the three can never desync.

## The two seams

Almost every extension goes through one of these.

**`engine.Engine`** — two methods, `Name()` and `Generate()`. A new backend
implements them and nothing else changes: the pipeline, the prompts and the
validation are identical whichever backend runs. `engine.Chain` orders backends
and fails over between them; the cursor is sticky so a dead provider is not
retried per paper, and half-open so a blip does not demote a whole queue.

**`pipeline.Event`** — a sum type on a channel. The TUI, the `--print` runner
and the `--json` runner are three consumers of the same stream, which is why
the pipeline has no idea a terminal exists. `TokenEvent` is dropped under
backpressure; every other event blocks, because losing a terminal event would
strand a caller waiting for an outcome that never comes.

## Where a change usually goes

| To change…                          | Edit                                                  |
| ----------------------------------- | ----------------------------------------------------- |
| What the model is asked for         | `prompt/` — and the matching rule in `rules/`         |
| What counts as a valid claim        | `claims/verify` — expect to update the mutation tests |
| What blocks a save                  | `validate/`                                           |
| The house vocabulary or length band | `rules/` only; both prompt and validator read it      |
| Support for another agent CLI       | `engine.Register`, or the `defaultProviders` table    |
| Another source format               | `internal/pdf.Extract`                                |
| Anything about ordering or failover | `pipeline/`                                           |

See [`docs/adr/`](adr/) for the decisions behind the shape, and
[`docs/SPEC-routing-and-resume.md`](SPEC-routing-and-resume.md) for the routing
and resume design — including an option deliberately not taken.
