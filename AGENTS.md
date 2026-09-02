# AGENTS.md

Invariants for AI-assisted contributions. Read this before changing anything;
it is shorter than the diff you were about to write.

Everything here also applies to humans. It is addressed to agents because
agents are fast enough to violate all of it before anyone notices.

## The one thing that must not break

**A claim reaches the writing prompt only if its quote occurs verbatim in the
source section it came from.** Every other rule in this file exists to protect
that one. A change that lets an unverified fact reach the writer is not a
trade-off to discuss; it is the product ceasing to be the product.

Related invariants, each pinned by a test, are listed in
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md#invariants). Do not weaken one to
make a test pass. If a test is in your way, the test is probably right.

## Before you claim to be done

The repository has gates, and "it compiles" is not one of them:

```console
make check          # fmt, vet, test, docs-lint
make cover          # the 98% coverage gate CI applies
golangci-lint run   # must be zero issues
```

State what you ran and what you observed. If you skipped something, say so.
A benchmark delta inside noise is not a win, a test that was never seen to
fail proves nothing, and "green on my machine" is not the full gate.

## Rules with teeth

1. **Never edit generated output.** Manpages, shell completions and the manual
   are generated from the source of truth. If the output is wrong, the
   generator is wrong. `.gen/`, `bin/` and `dist/` are not source.
2. **Do not hand-write a second copy of anything.** The completion flag list is
   derived from the help table, the manual from the repository's Markdown, the
   manpage from the CLI definitions. A comment promising two lists agree is not
   a mechanism; deriving one from the other is.
3. **`build/` is tracked** and holds release signing certificates. It is not a
   scratch directory. `rm -rf build` deletes real files.
4. **Coverage is a floor, not a target.** New code arrives with the test that
   would fail without it, in the same commit. "I'll add tests later" means no.
5. **The mutation gate on `claims` must stay at 100% efficacy.** A surviving
   mutant is a missing assertion, not a flaky tool.
6. **Comments say why, not what.** The codebase is written this way throughout;
   match it. A comment restating the line below it is noise.
7. **British English** in prose and comments.

## Security rules

`draft` puts text from documents the user downloaded into a prompt for an agent
running on their machine with their credentials.

- **Never add a flag that grants a provider tools.** `TestNoProviderGrantsTools`
  pins this. Not `--allow-all-tools`, not `--force`, not `--yolo`. `draft` only
  ever asks for text.
- **Never remove the sandbox.** Providers run in an empty temporary directory
  with a filtered environment. See
  [ADR 0004](docs/adr/0004-no-tools-granted-to-providers.md).
- **Untrusted text stays fenced.** Source documents, the ledger and the draft
  under review go through `prompt.Untrusted`.
- **A cached or resumed ledger is re-verified, never trusted.**

## Versioning and releases

- Conventional commits. A breaking change is marked `!` and explained.
- A change to *generated output* is breaking even when no signature moves; see
  [Stability guarantees](README.md#stability-guarantees).
- Do not tag a release, publish anything, or push to `main`. Open a pull
  request. Releases are a maintainer decision.
- Update `CHANGELOG.md` under `[Unreleased]` in the same change.

## Where to look first

| Question                           | File                                                     |
| ---------------------------------- | -------------------------------------------------------- |
| How does this work?                | [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)           |
| Why is it like this?               | [`docs/adr/`](docs/adr/)                                 |
| How do I run the gates?            | [DEVELOPMENT.md](DEVELOPMENT.md)                         |
| What is deliberately out of scope? | [When not to use draft](README.md#when-not-to-use-draft) |
| What is already known to be wrong? | [`docs/AUDIT-2026-09.md`](docs/AUDIT-2026-09.md)         |

## If you are unsure

Say so in the pull request instead of guessing. An honest "I could not verify
this on macOS" is worth more than a confident claim that turns out to be
untested — and the reviewer can check it in a minute.
