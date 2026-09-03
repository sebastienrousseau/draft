# Development

The single entry point for working on `draft`: toolchain setup, how to run
every CI gate locally, how the tests are laid out, and how a release is cut.

If a gate is green here it is green in CI — every job below is reproducible
with one command, and where CI pins a version this file names the same pin.

## Contents

- [Toolchain](#toolchain)
- [Runtime dependencies](#runtime-dependencies)
- [Everyday commands](#everyday-commands)
- [Reproducing every CI gate](#reproducing-every-ci-gate)
- [Test layout](#test-layout)
- [Where the time goes](#where-the-time-goes)
- [Release model](#release-model)

## Toolchain

| Tool             | Version                                | Why this version                                                                                                                                                                                                                |
| ---------------- | -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go               | 1.24 minimum, latest stable to develop | 1.24 is the declared floor in `go.mod` and is built and tested by the `msrv` CI job. See the [minimum-Go policy](README.md#minimum-go-policy).                                                                                  |
| `golangci-lint`  | v2.13.2                                | Pinned **together with** Go 1.26 in `ci.yml`. The linter embeds `go/types` from the toolchain it was built against, so a pinned linter against a floating toolchain breaks on every Go release. Bump the pair, never one alone. |
| Node (via `npx`) | any current                            | Only for `markdownlint-cli2`; fetched on demand.                                                                                                                                                                                |
| Python 3         | 3.9+                                   | `codespell` and `scripts/check-links.py`.                                                                                                                                                                                       |

No build step needs cgo: `CGO_ENABLED=0` is set for releases, which is what
makes the Linux binary static and portable.

## Runtime dependencies

`draft` shells out rather than linking parsers, which keeps the binary small
and portable at the cost of two documented dependencies:

| Tool                  | Needed for   | Install                                              |
| --------------------- | ------------ | ---------------------------------------------------- |
| `pdftotext` (Poppler) | PDF sources  | `brew install poppler` · `apt install poppler-utils` |
| `textutil`            | DOCX sources | macOS only, preinstalled                             |

Plus one backend: any supported agent CLI (online) or a running Ollama server
(offline). `draft --doctor` reports exactly what is present and what is not.

## Everyday commands

```console
make build      # ./bin/draft
make generated  # manpage + shell completions into bin/gen
make test       # go test ./...
make race       # go test -race ./...
make cover      # coverage + the same 98% gate CI applies
make bench      # benchmarks
make lint       # golangci-lint
make docs-lint  # markdown, spelling, intra-repo links
make check      # fmt, vet, test, docs-lint
make help       # every target

# The Unix install contract (GNU make reads GNUmakefile first, and it
# includes Makefile, so every target above still works):
make -f GNUmakefile install   PREFIX=/usr/local DESTDIR=/tmp/stage
make -f GNUmakefile uninstall PREFIX=/usr/local DESTDIR=/tmp/stage
```

The manpage and the shell completions are **generated from the CLI
definitions** (`draft --man`, `draft --completion <shell>`) and never
committed. A checked-in `.1` is a copy of the CLI that nothing keeps honest:
flags get added, the manpage does not, and the packaged documentation quietly
describes a different program.

## Reproducing every CI gate

Each row is a job in `.github/workflows/`. Run the command; if it passes, that
job passes.

| CI job                        | Local command                                       |
| ----------------------------- | --------------------------------------------------- |
| `build` → formatting          | `test -z "$(gofmt -s -l .)"`                        |
| `build` → vet                 | `go vet ./...`                                      |
| `build` → race                | `go test -race ./...`                               |
| `build` → coverage            | `make cover`                                        |
| `test (ubuntu/macos/windows)` | `go test ./...`                                     |
| `lint`                        | `golangci-lint run`                                 |
| `msrv (go 1.24)`              | `go build ./... && go test ./...` on Go 1.24        |
| `govulncheck`                 | `make vuln`                                         |
| `reuse`                       | `reuse lint`                                        |
| `docs-lint`                   | `make docs-lint`                                    |
| `regression` (benchmarks)     | see [Benchmark regressions](#benchmark-regressions) |
| `deep-quality` → fuzz         | `make fuzz` (`FUZZTIME=2m make fuzz` matches CI)    |
| `deep-quality` → mutation     | `make mutation`                                     |

Everything at once, in CI order:

```console
gofmt -s -l . && go vet ./... && go test -race ./... && make cover \
  && golangci-lint run && make docs-lint && make vuln && reuse lint
```

### Benchmark regressions

`benchmark.yml` fails a pull request when any metric regresses more than
**25%** against the merge base. Reproduce it:

```console
go test -run=NONE -bench=. -benchmem -count=8 ./... > head.txt
git stash && go test -run=NONE -bench=. -benchmem -count=8 ./... > base.txt && git stash pop
go run golang.org/x/perf/cmd/benchstat@latest base.txt head.txt
```

Use `-count=8`; fewer runs make benchstat report noise as significance.

## Test layout

Go convention: tests live beside the code they cover, not in a separate tree.

| Pattern             | What it holds                                                                  |
| ------------------- | ------------------------------------------------------------------------------ |
| `*_test.go`         | Unit tests, in the same package                                                |
| `example_test.go`   | Runnable examples, executed by `go test` and rendered on pkg.go.dev            |
| `bench_test.go`     | Benchmarks                                                                     |
| `fuzz_test.go`      | Fuzz targets, with their seed corpus in `testdata/fuzz/`                       |
| `hardening_test.go` | Regression tests for defects found by fuzzing or audit                         |
| `examples/`         | Standalone programs demonstrating the library; excluded from the coverage gate |

### The coverage gate is 98%, deliberately

Not 100%, and not a number chosen to be comfortable. The uncovered remainder
is dominated by `main()` and by error branches that need a failing syscall to
reach. Chasing the last two points would mean injecting seams into code that
does not otherwise need them, which trades real design for a number.

98% is high enough that a new untested branch is visible in the diff, and the
gate excludes `examples/` because those are demonstrations, not library code.

### The grounding corpus

`claims/testdata/corpus/` holds twelve sections with realistic model output.
Each case names the rule it guards, and the test pins how many candidate claims
survive the gate and how many are refused.

```console
make corpus       # against the recorded extractions; runs in CI
make corpus-live  # against a real backend; needs one, costs a call per case
```

The two halves catch different regressions, and neither substitutes for the
other:

- **`make corpus`** replays recorded extractions, so it is deterministic and
  runs on every push. It catches a change to *verification* — loosening the
  number check, for instance, makes the invented "34% improvement" in
  `03-number-absent-from-quote` survive.
- **`make corpus-live`** asks a real backend to extract from the same sources
  and compares against `baseline-live.json`. It catches the other half: a
  reworded `prompt.Claim` that quietly makes the model return fewer usable
  claims. Nothing about that is visible in a diff, and the cost only shows up
  as thinner articles weeks later.

**Run `make corpus-live` before merging any change to `prompt.Claim`.** It is
not in CI because GitHub's runners have neither an agent CLI nor an Ollama
server, and a workflow step that always skips is theatre rather than a gate.

The live comparison is against `baseline-live.json`, *not* the `verified`
counts in `corpus.json`. Those describe deliberately bad extractions recorded
to pin the gate; a real model produces good output and verifies more, so
comparing the two measures nothing. Baselines are specific to the engine and
model that recorded them and drift as models change, which is why the check
fails only on a collapse rather than on any difference.

### The grounding gate is mutation-tested

`claims` is the package that decides whether a claim is trustworthy, so line
coverage is not sufficient evidence — a test that runs a line without asserting
its behaviour still counts. `make mutation` requires **100% efficacy** on that
package: every mutant must be killed. Treat a surviving mutant as a missing
assertion, not a flaky tool.

## Where the time goes

Worth knowing before optimising anything. On a 14-section paper:

| Component                | Share                        |
| ------------------------ | ---------------------------- |
| Model latency            | ~99%                         |
| Provider process startup | 0.1–20% depending on the CLI |
| `pdftotext`              | ~0.1%                        |
| All `draft` Go code      | **<0.1%**                    |

Micro-optimising the Go code has no product effect. The wins are in call
count, caching and provider choice. See [`docs/AUDIT-2026-09.md`](docs/AUDIT-2026-09.md).

## The documentation site

The manual is published to <https://sebastienrousseau.com/draft/> by the
`docs` workflow on every push to `main`. Its pages are assembled from this
repository's Markdown by `scripts/build-manual.py`; nothing is authored in the
site, so a chapter cannot disagree with the file it came from.

```console
make manual        # build into .gen/site
make manual-serve  # live reload while editing
```

### The site is served behind a Content-Security-Policy

The custom domain is fronted by Cloudflare, which adds a domain-wide CSP with
an **allowlist of image origins**. GitHub Pages cannot set response headers, so
this rule lives in the Cloudflare configuration and not in this repository.

The practical consequence: **an external image added to the documentation will
render on GitHub and be silently blocked on the site.** The README's shields.io
badges hit exactly this on the day the manual first published — the markup was
correct and the images returned `200 image/svg+xml`, but the browser refused to
load them because `img.shields.io` was not in `img-src`. It has since been
added, so the badges render; the constraint itself has not gone away.

The failure gives you nothing to go on: no server-side error, no CI failure,
just an empty space where the image should be. If you add an image from a new
origin, add that origin to the CSP's `img-src` too. To check what the site
currently allows:

```console
curl -sSI https://sebastienrousseau.com/draft/ | grep -i content-security-policy
```

And to confirm a page's external resources are actually permitted, compare the
origins it references against that header — a blocked resource produces no
server-side error, only an empty space and a console message.

---

## Release model

Versions are `0.0.x` until `0.0.999`; the CLI surface and output layout are
covered by the [stability policy](README.md#stability-guarantees).

1. Land the change through a pull request with CI green.
2. Update `CHANGELOG.md` — Keep a Changelog form, one heading per release.
3. Tag `vX.Y.Z` and push it.
4. `release.yml` builds six binaries, generates SBOMs, signs the checksum file
   with keyless Sigstore, notarizes the macOS binaries, and attests build
   provenance. It is gated on the protected `release` environment.

Before the first use of new release machinery, run the workflow's dry-run mode
(`workflow_dispatch` with `dry_run: true`), which exercises everything except
publishing.

### Recovering a failed release

If `release.yml` fails partway, some artifacts may already be published while
later steps — provenance attestation, the macOS installer, the verification
job — never ran. A re-run has to start from a commit whose workflow contains
the fix, and **the tag decides which workflow file runs**, so fixing `main` is
not enough on its own:

```console
# 1. Land the fix on main and confirm CI is green.
# 2. Remove the incomplete release, keeping nothing stale behind.
gh release delete vX.Y.Z --yes            # the tag survives this
# 3. Move the tag onto the fixed commit and re-push, which re-triggers the
#    workflow. Verify the signature before pushing, since a re-tag re-signs.
git tag -d vX.Y.Z && git push origin :refs/tags/vX.Y.Z
git tag -a vX.Y.Z -m "..." && git tag -v vX.Y.Z
git push origin vX.Y.Z
```

Check the download counts first (`gh release view vX.Y.Z --json assets`). This
is only safe while nobody holds the old artifacts: rebuilt binaries are not
guaranteed byte-identical, so anyone who already downloaded would see a
checksum mismatch. If there are downloads, ship the fix as the next version
instead.

Verifying a release is documented in [`docs/RELEASE_SECURITY.md`](docs/RELEASE_SECURITY.md).
