# Contributing to draft

Thanks for your interest in improving `draft`. This guide covers the workflow,
the quality gates, and the conventions the project holds to.

## Getting started

```sh
git clone https://github.com/sebastienrousseau/draft
cd draft
make build      # compile to ./bin/draft
make test       # run the suite
```

You need Go 1.25+. Runtime tools (`pdftotext`, a session CLI or Ollama) are only
needed to actually draft; the test suite fakes them and needs neither a network
nor an LLM.

## Development workflow

1. Branch from `main` (e.g. `feat/…`, `fix/…`, `docs/…`).
2. Make your change with a test that fails before and passes after.
3. Run the full gate locally (below).
4. Open a pull request against `main`. The PR template lists the checklist; CI
   enforces it.

## Quality gates

Every change must keep these green — CI runs them and the branch ruleset
requires them:

```sh
make fmt        # gofmt -s -w .   (CI checks `gofmt -s -l .` is empty)
make vet        # go vet ./...
make lint       # golangci-lint run   (config in .golangci.yml)
make test       # go test ./...
make cover      # coverage must stay >= 98% of app/library statements
make bench      # benchmarks (for performance-sensitive changes)
make fuzz       # fuzz every untrusted-input parser
make mutation   # mutation-test the grounding gate
```

- **Tests.** Add or update tests in the same commit as the behaviour change.
  Coverage is enforced at **≥98%** (demo `examples/` are excluded). The pipeline
  is tested end to end against a deterministic fake `Engine`; provider CLIs are
  faked via the `TestHelperProcess` pattern, so you never need a real agent.
  Pull requests that change Go code compare benchmarks against their base, and
  the scheduled deep-quality workflow runs extended fuzzing and mutation tests.
- **Docs.** Every exported symbol carries a doc comment (revive's `exported`
  rule is enforced). User-facing changes update the `README.md` and `--help`.
- **Changelog.** Note user-facing changes in `CHANGELOG.md` under the unreleased
  or current `0.0.x` heading.

## Commit and PR conventions

- **Conventional commits** — `feat:`, `fix:`, `docs:`, `ci:`, `refactor:`, etc.
- **Signed commits are required.** CI and the branch ruleset verify signatures;
  configure signing (`git config commit.gpgsign true`) before you push.
- Keep PRs focused; one logical change per PR where practical.

## Adding a provider

Session providers live in `internal/engine/providers.go`. A new entry needs its
headless invocation (derived from the CLI's `--help`) and should be marked
`Experimental: true` until its article output is verified end to end — auto mode
only uses non-experimental providers.

## Project structure

`draft` — the CLI, the grounding and claim gate, verification, local execution,
and the importable Go packages — is and stays open source under **MIT OR
Apache-2.0**, with no account or licence server. That is the product's promise,
not a trial.

Any hosted or enterprise components (managed attestation, signing/key
management, organisation policy and audit) are developed in a **separate
repository** and are not part of this tree. Contributions here are to the open
core; the boundary is deliberate, so a contribution never lands you in
proprietary territory by accident.

## License and Developer Certificate of Origin

`draft` is licensed **MIT OR Apache-2.0**, and by contributing you agree your
contributions are licensed under those same terms (inbound = outbound).

Every commit must also be **signed off** under the
[Developer Certificate of Origin](DCO): a line-by-line certification that you
wrote the change, or have the right to submit it, under the project's licence.
It is a lightweight attestation of origin — not a copyright assignment — and it
keeps the project's provenance clean for everyone downstream.

Add the sign-off automatically with `-s`:

```sh
git commit -s -m "feat: …"
```

which appends a trailer matching your commit author:

```
Signed-off-by: Your Name <you@example.com>
```

CI checks that every commit in a pull request carries it. (This is distinct
from the cryptographic commit *signature* the branch ruleset also requires —
you need both: `git commit -s -S` once signing is configured.)
