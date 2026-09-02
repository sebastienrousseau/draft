# Governance

How decisions get made, written down because "ask the maintainer" is not a
governance model — it is the absence of one, and it leaves contributors
guessing about which changes are welcome.

## Current model: single maintainer (BDFL)

`draft` is maintained by [Sebastien Rousseau](https://github.com/sebastienrousseau),
who has final say on scope, design and releases. This is stated plainly rather
than implied, so nobody invests effort on a change that was never going to land.

The maintainer commits to:

- Responding to security reports within the SLA in [SECURITY.md](SECURITY.md).
- Giving a reason when a contribution is declined, not just a close.
- Recording decisions that will be questioned later in
  [`docs/adr/`](docs/adr/), with the alternatives and what the choice costs.
- Keeping `main` releasable: CI green at every merge.

## What gets accepted

Weighted roughly in this order:

1. **Does it preserve the grounding guarantee?** A change that lets an
   unverified fact reach the writer is declined regardless of its other merits.
   The invariants are listed in [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).
2. **Is it in scope?** `draft` turns research documents into grounded drafts.
   [When not to use draft](README.md#when-not-to-use-draft) is the boundary,
   and it is meant to be a real boundary.
3. **Is it enforced?** New behaviour comes with the test that would fail
   without it. A standard that is not a CI gate decays.
4. **Does it explain itself?** Comments here say *why*, not *what*. A change
   that would puzzle a reader in six months should carry its reason.

## Proposing a significant change

Anything that alters the CLI contract, the output format, the grounding rules
or a dependency: open an issue first describing the problem before the
solution. For a load-bearing decision, an ADR drafted from
[`docs/adr/template.md`](docs/adr/template.md) is the fastest route to a yes or
a well-reasoned no.

Small fixes need no ceremony — open the pull request.

## Releases

The maintainer cuts releases; there is no fixed cadence. The process, and what
counts as a breaking change, are in
[DEVELOPMENT.md](DEVELOPMENT.md#release-model) and
[Stability guarantees](README.md#stability-guarantees).

## Changing this document

Governance changes the same way code does: by pull request, with a reason.

## If the project is not maintained

Should the maintainer become unable to continue, the intent is to say so in the
README rather than let the repository look active while it is not. The dual
MIT / Apache-2.0 licence means a fork needs nobody's permission.
