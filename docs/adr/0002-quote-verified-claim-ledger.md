# 0002. Ground by verifying quotes before writing, not by checking after

- **Status:** Accepted
- **Date:** 2026-07

## Context

The field's approach to hallucination is largely post-hoc: generate, then score
the result against its sources with a learned metric (AlignScore, RAGAS) or a
small verifier (MiniCheck, FactCG). Those are good at *detecting* a problem.

Detection is the wrong shape for a drafting tool. By the time a score is low
the expensive call has already happened, and the remedy is either shipping a
flagged draft or paying for the whole thing again.

## Decision

Mine claims first, and let a claim into the writing prompt only if its
`SOURCE_QUOTE` is a verbatim substring of the section it came from, after
normalising case, whitespace and smart quotes. Every number in the claim must
also appear in that quote. The writing model is then told the ledger is its
only permitted source of facts.

## Alternatives considered

- **Post-hoc scoring.** Detects rather than prevents, and a learned score is a
  threshold to argue about; an exact substring match is not.
- **Retrieval at writing time.** The model would choose what to quote while
  writing, which is precisely the step that invents.
- **Asking the model to self-check.** The same model that fabricated a number
  is not independent evidence about it.

## Consequences

- The strongest guarantee in the product: a fabricated fact cannot reach the
  writer, because it never entered the ledger.
- **Verification is cheap and mechanical** — no second model, no threshold — so
  it can run on every claim without affecting the cost of a run.
- It is strict, and strictness costs recall. A correct claim whose quote was
  paraphrased by the extracting model is dropped. Typical runs drop a
  meaningful fraction of candidates, which is the intended trade.
- Correctness now depends on extraction fidelity, which is what makes
  [ADR 0001](0001-plain-text-extraction.md) load-bearing rather than an
  implementation detail.
- The gate is security-critical, so line coverage is not sufficient evidence:
  `claims` is mutation-tested at 100% efficacy.
