# 0009. Add an opt-in semantic second gate over the verified claims

- **Status:** Accepted
- **Date:** 2026-09-11

## Context

The verbatim gate ([0002](0002-quote-verified-claim-ledger.md)) proves a
claim's quote occurs in the source and every number in the claim occurs in the
quote. It is deterministic, model-free, and honest that it checks *wording, not
meaning*: a quote can be present, and its numbers match, while the claim reads
the opposite direction, states a hedge as settled, or is about something else.
That semantic gap is the one thing the gate documents it cannot close.

## Decision

Add an opt-in second pass (`--second-gate` / `DRAFT_SECOND_GATE`): after the
verbatim gate, a local model judges whether each verified quote actually
*supports* its claim, and unsupported claims are dropped before writing. It is
off by default; the verbatim gate stays the primary, always-on check.

## Alternatives considered

- **Make it the default / replace the verbatim gate.** Rejected: a model-based
  judgement is non-deterministic and can err; it must never be the thing that
  decides grounding. It is additive on top of the deterministic gate, not a
  substitute.
- **A bundled NLI model (MiniCheck-class).** Rejected for now as a heavy
  dependency; the pass runs through the configured engine seam instead, so a
  user points it at whatever model they already have.
- **Fail-closed on model error.** Rejected: a flaky or offline verifier would
  then silently thin a run's grounded material. The pass is fail-open — a model
  error, a cancelled context, or an unreadable verdict keeps the claim.

## Consequences

- It costs one extra model call per verified claim when enabled, so it is a
  deliberate opt-in, not free.
- Its quality depends on the model behind the engine seam, which is why it is
  additive and fail-open rather than authoritative.
- The verbatim gate's guarantee is unchanged whether it is on or off.
