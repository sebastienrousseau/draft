# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/), and versions use a `0.0.x`
series until `0.0.999`.

## [Unreleased]

## [0.0.33] - 2026-09-03

### Security

- **Bounded the blast radius of a crafted source document.** `draft` puts a
  downloaded PDF's text into a prompt for an agent running with your
  credentials, which is the canonical indirect-prompt-injection shape. Provider
  subprocesses now run in a fresh empty temporary directory (removed when the
  call returns) rather than your working directory, so a source cannot reach an
  agent that has loaded the `CLAUDE.md`, `AGENTS.md`, `.mcp.json` or project
  settings sitting there. `DRAFT_*` is stripped from the child environment.
- **Removed every tool grant.** `copilot --allow-all-tools` and
  `cursor-agent --force` (its own `--help` calls that an alias for `--yolo`,
  "Run Everything") are gone. `draft` only ever asks for text. A test pins the
  invocation table against a list of known tool-granting flags.
- **Fenced untrusted text.** Source documents, the claim ledger and the draft
  under review are wrapped in a nonce-delimited block stating that the contents
  are data, never directions. The nonce is fresh per call, so a document cannot
  close its own block and continue as the operator. The ledger is fenced too: a
  `SOURCE_QUOTE` is verbatim source by construction.
- **Kept prompts out of `argv` for two more providers.** `goose` is driven over
  stdin (`run -i -`) and `grok` through a `0600` prompt file, per each CLI's own
  `--help`. Only `copilot` and `agy` still take an argument, and a test pins
  that exception list.

### Fixed

- **`enforceStyle` no longer rewrites quotations.** It ran its banned-vocabulary
  replacers over the whole draft, so `The paper states: "we leverage a robust
  seamless pipeline"` became `"we use a strong smooth pipeline"` — attributing
  to a source words it never wrote. Code and quoted spans are now excluded, and
  the banned-vocabulary check skips them too, so the repair pass is never asked
  to satisfy a rule it refuses to touch.
- **Prompts are cut on rune boundaries.** `ContinueWriting`, `prompt.clip` and
  the template excerpt all sliced at raw byte offsets; on non-ASCII sources that
  produced invalid UTF-8, and `claims.Verify` rejects any quote containing
  U+FFFD. Lifting the existing helper out of `internal/pdf` also surfaced an
  off-by-one in it: a document opening with a multi-byte rune was cut mid-rune.
- **Shell completion is derived from the help table.** The two lists had drifted
  by four flags despite a comment promising they could not.
- **`ollama serve` is reaped.** A started process that is never waited on stayed
  a zombie for as long as `draft` lived.

### Added

- **`--out` and `--sources-dir`** (`DRAFT_DRAFTS_DIR`, `DRAFT_SOURCES_DIR`).
  Both trees were hardcoded under the home directory with no flag and no
  variable, which made `draft` unusable from CI, a container, or as a library
  whose caller chooses where output goes.
- **`--doctor`.** Checks Poppler, the installed backends, Ollama, the resolved
  paths and their writability, and the per-kind engine routing. Every
  requirement was previously discovered at failure time.
- **`--strict-numbers` / `DRAFT_STRICT_NUMBERS`.** Promotes "numbers not found
  in any claim" from a warning to a blocking error. Opt-in for one release while
  the false-positive rate is measured; ordered-list markers and four-digit years
  are excluded when blocking, and still reported when advisory.
- **A run manifest on `--json`,** plus `"schema": 1`. Records the draft version,
  model, a hash of the extraction instructions, the ledger digest and the
  SHA-256 of every source file.

- **A Unix install contract.** `GNUmakefile` implements `make install` and
  `make uninstall` honouring `PREFIX` and `DESTDIR`, installing to FHS
  paths, and CI verifies the staged tree against the exact list
  `docs/packaging.md` promises packagers.
- **`draft --man`** generates a roff manual page from the same tables
  `--help` renders, so the packaged documentation cannot describe a
  different program than the binary beside it.
- **Native packages.** Releases now carry `.deb`, `.rpm` and `.apk` built
  from the same binaries the archives do, and the archives themselves now
  contain the manpage, shell completions and licence texts rather than a
  bare binary.
- **A release dry run** (`workflow_dispatch`), so new release machinery can
  be exercised before a tag makes its failure permanent.

### Changed

- **Claim extractions are cached by content** rather than by date. The only
  reuse was `--resume`, whose ledger is named for today's date and the first
  source's filename, so redrafting the same paper tomorrow re-paid the whole
  cost — 80-95% of a run's wall clock. Measured on a one-section source, the
  second run's extract phase went from 15,108 ms to 1 ms with an identical
  ledger digest. A cached entry is still re-verified against the freshly read
  source, so a stale one can only ever yield fewer claims, never an ungrounded
  one. `--no-cache` and `--clear-cache` control it.
- **A demoted engine recovers.** The fallback cursor was permanent for the life
  of a `Runner`, so one flaky moment on the first paper demoted a forty-paper
  queue to the local model for its entire life. The chain now re-probes after
  15 minutes.
- **Ollama concurrency follows the server.** `OLLAMA_NUM_PARALLEL` is read, and
  the measured floor of 2 is no longer a ceiling.
- **Near-duplicate paragraphs are repaired without regenerating.** A rule
  violation cost a full rewrite — the most expensive call in a run — even when a
  deterministic edit could settle it.
- **BREAKING: `engine.Providers` is a function, not a slice.** It was an
  exported mutable table that callers were invited to append to, which races
  every read of it in a concurrent program. Use `engine.Providers()` to read and
  `engine.Register` to extend.

- Bumped `actions/attest-build-provenance` v4.1.1 -> v4.2.2 (both call
  sites in `release.yml`).

- **Documentation is gated.** markdownlint, codespell, an intra-repo link
  and heading-anchor check, and a version-consistency check all run on every
  push. The repository had 615 markdownlint findings and no gate; it now has
  none.
- **Every exported name must carry a doc comment** (revive's `exported`
  rule), and a new `api surface` job reports how the public API moved since
  the last release.
- Bumped `anchore/sbom-action/download-syft` v0.24.0 -> v0.24.2.

### Documentation

- **`docs/AUDIT-2026-09.md`** — a full architecture, performance, security and
  product audit against 2026-2027 market context, with the roadmap these changes
  implement.

- **A rendered user manual, a development guide and decision records.**
  `DEVELOPMENT.md` maps every CI job to the one command that reproduces it
  locally; `docs/ARCHITECTURE.md` describes the package graph, the five
  phases and the invariants each has a test for; `docs/adr/` records five
  decisions with their alternatives and what each costs. The manual is
  assembled from these same files, so a chapter cannot disagree with the
  document it came from.
- **`docs/packaging.md` and `pkg/`** for distribution maintainers, plus
  `supply-chain/` recording what is depended on and how it is pinned.
- **`GOVERNANCE.md`, `SUPPORT.md`, `AGENTS.md` and `CITATION.cff`.**

## [0.0.32] - 2026-08-04

### Security

- **Protected release credentials.** Apple signing and notarization secrets are
  isolated in an approval-gated GitHub environment restricted to version tags.
- **Notarized universal installer.** Releases now include a Developer ID signed,
  Apple-notarized and stapled macOS package, with provenance and signed checksums.
- **Credential lifecycle automation.** Weekly checks warn at 60 and 30 days
  before certificate expiry and before the 90-day API-key rotation deadline.
- **Independent release verification.** A clean macOS runner verifies every
  checksum, Sigstore bundle, provenance attestation, Apple signature, package
  ticket, architecture, and executable version after publishing.

## [0.0.31] - 2026-08-04

### Security

- **Native macOS trust.** Release binaries for Intel and Apple silicon are now
  signed with a Developer ID Application certificate and notarized by Apple
  before archiving.

## [0.0.30] - 2026-07-30

### Added

- **Interactive LLM Provider Selection.** Running `draft` without specifying an
  explicit `--engine` flag now presents a branded interactive LLM selection screen
  prior to starting generation. Users can select from `auto`, session CLIs (`claude`,
  `agy`, `codex`, `copilot`, `cursor-agent`, `grok`, etc.), or local offline `ollama`.
  Each provider indicates installation status (`[installed]`, `[local / offline]`, `[auto]`).
  Explicit `--engine <name>` usage and non-interactive modes bypass selection directly.

### Changed

- **Unified Brand Wordmark.** Set the brand wordmark to lowercase `draft` across
  all CLI surfaces, dashboard header renders, and library constants to match the
  website (`draftlib.com`) and binary command name. Pinned the wordmark shape in unit
  tests to prevent drift.

### Quality & Governance

- **Install with Homebrew or mise.** Release archives now have documented,
  first-class installation paths through `sebastienrousseau/tap/draft` and
  mise's native GitHub backend, alongside `go install` and `make install`.
- **Deep quality automation.** Pull requests compare benchmarks with their base
  revision and fail on significant regressions above 25%. A daily workflow runs
  extended native fuzzing and mutation-tests the security-critical grounding
  gate at 100% efficacy and mutant coverage.
- **Repository Audit (10/10 in all categories).** Full audit verified clean zero-issue
  `golangci-lint`, 98.1% test coverage (exceeding the 98% gate), `go test -race` clean,
  `govulncheck` clean, REUSE/SPDX license compliance, and strict versioning policy
  (incrementing by 0.0.1 up to `v0.0.999` before `v0.1.0`).

## [0.0.29] - 2026-07-30

### Added

- **Per-stage engine routing.** `DRAFT_EXTRACT_ENGINE`, `DRAFT_WRITE_ENGINE`
  and `DRAFT_EDIT_ENGINE` (with `--extract-engine` / `--write-engine`) send
  each stage to its own backend. The workload is lopsided — a dozen cheap,
  mechanical extraction calls per paper against one quality-critical write —
  so `DRAFT_EXTRACT_ENGINE=ollama DRAFT_WRITE_ENGINE=claude` cuts session
  usage roughly thirteenfold while keeping the best available writer. Each
  stage keeps its own fallback chain and cursor, so extraction failing over to
  Ollama does not drag writing down with it.
- **`--resume`.** Extraction is 80-95% of a run's wall clock, and a failed
  write phase used to discard all of it — even though the verified ledger was
  already on disk. `--resume` re-verifies that ledger against the freshly
  sectioned sources and skips straight to writing. Re-verification is
  mandatory: a resumed ledger is trusted because it still passes the same
  gate, not because we wrote it, so resume cannot weaken grounding.
- **`--dry-run`.** Reports sources, section count, per-stage routing, the
  estimated model-call count and whether a resumable ledger exists — in the
  deterministic ~110 ms of a run, without calling a model. It exercises the
  real resolve and sectioning path, so a clean plan is evidence the sources
  are readable rather than a guess.
- **An extraction ETA** on the progress line, from a running mean of completed
  sections rather than an extrapolation from the first (which settles the
  engine chain and warms the model, and so overstates badly).

- `DRAFT_CALL_TIMEOUT` bounds a single generation call (default 1800s, `0`
  disables), so a wedged provider CLI cannot hang a run forever. The Ollama
  backend uses its own HTTP client with dial and response-header timeouts
  rather than the timeout-free `http.DefaultClient`.
- `engine.Validate` reports an unknown engine name, which `Chain` cannot.
- `DoneEvent` carries `Duration` and per-phase `Timings`, surfaced in `--json`
  as `duration_ms` and `phases_ms`.
- `WarnEvent` distinguishes a non-fatal problem from ordinary progress, and
  `--json` records them per job as `warnings`.
- `Config.Warnings` reports configuration that was recovered from rather than
  applied silently; every numeric environment variable is now clamped at both
  ends instead of only floored.
- A README for each of the eight importable packages, with a runnable quick
  start and an API table.

### Fixed

- **Two papers drafted on the same day overwrote each other's claim ledger.**
  The filename was date-only; it is now derived from the job's sources. This
  lost the fact-checking artefact `--keep-artifacts` exists to preserve.
- **A dead provider was retried once per paper.** The `Runner` was constructed
  per job, resetting the fallback cursor every time, so a queue of twenty
  papers re-tried and re-reported every dead backend twenty times. One Runner
  now serves the queue and keeps the backend it settled on.

- **`--review` did not check faithfulness.** It gated only on the house style
  rules, so a surgical edit could introduce an ungrounded number or an
  unsupported metric into a finished article — and `factual correction` is an
  allowed edit reason. Both write paths now run `validate.Errors` *and*
  `validate.Faithfulness`.
- **A stalled consumer could wedge the pipeline.** `Runner.emit` performed an
  unconditional blocking send while its doc comment claimed the opposite, so a
  dashboard that quit mid-run left the goroutine blocked forever. Sends now
  race the run's context, and `TokenEvent`s are dropped rather than applying
  backpressure — a slow renderer must slow the preview, never the generation.
- **Truncation was only detected for Ollama.** Session providers never set
  `Result.Truncated`, so the continuation machinery was dead code for them and
  a length-limited stop surfaced much later as a rule violation costing a full
  rewrite. The stream-json stop reason is now parsed, and an ending that does
  not close a sentence is treated as a truncation for every backend.
- **Cancelling a run walked the rest of the engine chain**, failing each
  remaining backend in turn and logging a misleading fallback for each.
- **`--frontmatter` could leave an article set disagreeing with itself.** The
  three files were written with three sequential `os.WriteFile` calls, so a
  failure after the first left the set desynced. They are now staged and
  published as a unit, and `--review` writes through a temporary file and a
  rename so an interrupted write cannot destroy the original draft.
- **Section splitting could produce invalid UTF-8.** With no paragraph or
  sentence boundary to cut on, the fallback sliced at a fixed byte offset and
  split multi-byte runes in half.
- **A misspelled `--engine` silently used Ollama.** `draft --engine claud` now
  exits 2 and lists the valid names.
- **Drafts could be written to the wrong directory.** When `os.UserHomeDir`
  failed — routine under systemd, cron and in containers — the error was
  discarded, leaving the Sources and Drafts paths relative to whatever
  directory the process started in. They are now always absolute.
- **`uniquePath` could loop forever** on a directory it could not stat into.
- **Output filenames are claimed with `O_EXCL`** instead of a check-then-write,
  and the search for a free name is bounded.
- **`validate.Errors` now enforces `rules.MaxWords`.** The writing prompt asks
  for a 500–3000 word band and `rules` declares it, but only the floor was
  checked, so an over-long draft was told one thing and held to another. A
  newly generated draft that runs long is now rewritten rather than saved.
  `--review` is unaffected: it judges the edit, not the article (below).
- **`--review` no longer fails on a violation it did not introduce.** It
  operates on a file the user already has, which may predate a rule, exceed
  the length band, or read as ungrounded against a ledger mined from whichever
  sources were supplied today. Blocking on any of that made an existing article
  permanently unreviewable for a reason the review had nothing to do with. The
  gate now compares the article before and after the edit and fails only on
  what the edit added; pre-existing problems are reported as warnings.
- **Read errors during streaming are no longer swallowed**, so a broken pipe
  cannot produce a half-written article that looks complete.
- **Failures are reported honestly.** A claim ledger or rescued draft that
  could not be written is no longer logged as saved.
- **`pdftotext` failures surface the tool's own message** instead of
  `exit status 1`.

### Security

- **A fabricated claim could pass the grounding gate using invalid UTF-8.**
  Quote-to-source comparison lowercases both sides, and `strings.ToLower` maps
  every invalid UTF-8 byte to U+FFFD — so two *different* invalid byte
  sequences normalised to the same text and a quote could match a source it
  does not occur in. That is a bypass of the one invariant the claim ledger
  rests on. `claims.Verify` now rejects a quote that is not valid UTF-8 or that
  contains a replacement character; accented and CJK quotes are unaffected.
  Found by `FuzzParse`, and its input is kept as a corpus seed.
- **Prompts now go over stdin for `codex` and `cursor-agent`.** A prompt passed
  as a command-line argument is visible in a process listing for the duration
  of the call, along with the source excerpts it quotes. Stdin delivery was
  confirmed by running each CLI; `copilot`, `agy`, `grok` and `goose` were
  confirmed *not* to read stdin and keep argument delivery.
- **`OLLAMA_HOST` is validated.** A value that is not a valid `http`/`https`
  URL is refused rather than concatenated into a request URL, and a host that
  is not loopback is reported — a remote Ollama means prompts and verbatim
  source text leave the machine.
- **Extraction helpers are given absolute paths.** A source file named `-x.pdf`
  would otherwise be parsed as a flag by `pdftotext` or `textutil`. Their
  output is also capped, and source files are size-limited.

### Changed

- `make cover` now runs the same coverage gate CI does. It previously measured
  only `./internal/...` and `./cmd/...`, omitting every library package while
  describing itself as covering them.
- **`FuzzParse` asserts the contract the gate actually has.** Its invariant
  required a quote to appear byte-for-byte in the source, but quote matching is
  deliberately insensitive to runs of whitespace — a sentence wrapped across
  lines, or split by a column gutter, is still present in the paper it came
  from, and rejecting it would drop true claims from every real PDF. The
  invariant now folds whitespace, case and curly quotes, written independently
  of the production code so it tests the property rather than restating the
  implementation, and `TestGroundedInStillRejectsFabrication` pins that the
  looser bar still refuses invented content.

## [0.0.28] - 2026-07-29

### Changed

- **The README has been rewritten.** It now follows the structure used across
  these projects — grouped contents, a capabilities section, collapsible
  reference material, and an honest comparison — in a plainer, more direct
  voice. Claims lead with what the tool does for you; the numbers behind them
  follow.

### Fixed

- **Two stale statements in the README.** "How it works" still described
  extraction as `pdftotext -layout`, which 0.0.27 replaced with reading-order
  extraction, and the limitations section still pointed at `internal/rules`
  and `internal/validate`, which became public packages in 0.0.18. Also
  documents `--json`, `--completion` and `DRAFT_SHOW_LOGO`, which had no entry
  in the flag or environment tables.

## [0.0.27] - 2026-07-28

### Fixed

- **Two-column papers are no longer read across the columns.** Extraction
  passed `-layout` to `pdftotext`, which preserves the *visual* arrangement:
  on a two-column paper the left and right columns were spliced onto shared
  lines, so sentences broke mid-thought and merged with unrelated text. Since
  a claim survives verification only when its `SOURCE_QUOTE` appears verbatim
  in the source, this quietly undermined grounding on exactly the documents
  the tool is built for. Poppler's default reading order is now used. Measured
  on a real corpus, spliced lines fell from 158 and 59 on two papers to **zero
  on all of them**.
- **A contents listing no longer truncates the whole paper.** Sectioning cut
  the document at the *first* line matching `references|bibliography|
  acknowledgements|appendix`, which in a paper with a contents page is the
  entry "References … 4" in the front matter, not the bibliography. A 62-page
  paper was reduced to its first 8kB — the claim ledger was built from the
  table of contents. The last such heading is now used, which also keeps the
  degenerate case right: a document that is only a bibliography still yields
  no sections. The same paper now retains **97.3% of its text, up from 3.9%**.

### Added

- **A clear diagnosis for PDFs with no text layer.** A scan or image export
  previously produced empty text and a late, vague failure. `pdf.ErrNoTextLayer`
  now explains what happened and what to do — run OCR, or supply a text source.
- **Real PDF fixtures.** `internal/pdf/testdata` holds a generated two-column
  paper — complete with a contents listing naming "References" in its front
  matter — and a page with no text layer, both built by a committed script so
  they are reproducible and license-clean. Regression tests assert that no
  output line carries text from two columns, that body claims survive
  sectioning, that the real bibliography is still dropped, and that a
  text-less PDF is diagnosed.

## [0.0.26] - 2026-07-28

### Fixed

- **Release signatures are now discoverable.** The cosign bundle shipped as
  `checksums.txt.bundle`, an extension no tooling recognises — the OpenSSF
  Scorecard scored Signed-Releases 0 despite the release being signed. It is
  now `checksums.txt.sigstore.json`, the canonical Sigstore bundle name.
- **Workflow tokens are read-only by default.** `release.yml` requested
  `contents`, `id-token`, and `attestations` write at the workflow level,
  so every step ran with them. Permissions are now `read-all` at the top and
  widened only on the job that publishes.

### Added

- **CodeQL analysis** runs on push, pull request, and weekly, with the
  `security-and-quality` query suite.

## [0.0.25] - 2026-07-28

### Added

- **OpenSSF Scorecard analysis and badge.** A weekly (and on-push) workflow
  publishes the repository's supply-chain score, so the signing, SBOM,
  provenance, pinned-action and vulnerability-scanning work is visible
  rather than implied.
- **A module overview on pkg.go.dev.** `doc.go` explains the grounding
  guarantee, maps the packages, and states the 0.0.x stability contract, so
  the module's landing page says what the tool is for.

### Changed

- **`cmd/draft` coverage** rose from 88.6% to 91.1%, covering the `--json`
  failure record (a failed job must report `ok:false` with an error rather
  than claiming success), the version fallback, and the `max` helper.

## [0.0.24] - 2026-07-28

### Added

- **Signed releases with SBOMs and build provenance.** Every archive now
  ships a CycloneDX SBOM, `checksums.txt` is signed keylessly with Sigstore
  cosign (published as `checksums.txt.sigstore.json`, carrying both the signature
  and the signing certificate), and GitHub build provenance is attested for
  each artefact. Verify with:

  ```sh
  cosign verify-blob --bundle checksums.txt.sigstore.json \
    --certificate-identity-regexp 'https://github.com/sebastienrousseau/draft/.*' \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    checksums.txt
  ```

- **`--json`** runs headless and emits one JSON object per job (JSON Lines)
  on stdout — source, output path, engine, mode, word count, and status —
  leaving stderr for human progress, so runs can be piped into other tools.
- **`--completion <bash|zsh|fish>`** prints a shell completion script,
  completing flags, engine names, and file arguments.
- **Runnable examples on pkg.go.dev** for `config`, `frontmatter`,
  `pipeline`, `prompt`, and `rules`, so every public package now documents
  itself with compiled, verified code.
- **An API stability note** in the README: while the module is `0.0.x` the
  Go API may change between releases, the CLI surface is the stable one, and
  breaking changes are always listed in this file.

### Changed

- **The version has one source of truth.** It was a hand-maintained literal
  in `cmd/draft/main.go` *and* an ldflags injection — the duplication is how
  v0.0.22 once shipped mislabelled. It is now derived from the build info,
  overridden by ldflags for releases, and `make build`/`make install` stamp
  it from `git describe`, so a local build reports exactly what it is.

### Fixed

- **`rules` coverage** rose from 81.1% to 94.6%: `MetricForms`, which decides
  whether an abbreviation and its expansion count as the same metric during
  the faithfulness check, had no test at all.

## [0.0.23] - 2026-07-28

### Added

- **Vulnerability scanning in CI.** A `govulncheck` job now fails the build on
  any known vulnerability reachable from this code, and `make vuln` runs the
  same check locally. This was added because a scan found GO-2026-5856
  (`crypto/tls`) reachable from `engine/ollama.go` via `http.Client.Do`, with
  nothing in CI to catch it.
- **Fuzz targets for every untrusted-input parser.** `claims.Parse`,
  `frontmatter.Split`, `frontmatter.ExtractMetadata`, and the review-mode
  `parseSurgicalEdits` are now fuzzed against invariants rather than mere
  absence of panics: a surviving claim's quote must appear verbatim in the
  source (the grounding guarantee), splitting must be stable and lossless,
  metadata must stay valid UTF-8 with a URL-safe bounded slug, and an
  accepted edit set must never empty or corrupt a draft. `make fuzz` runs
  them all.
- **An MSRV job** builds and tests on Go 1.24, so the minimum version the
  README advertises stays verified now that the other jobs track `stable`.

### Changed

- **Builds and releases use a supported Go toolchain.** Every job pinned
  `go-version: "1.24"`, but Go only patches its two most recent releases, so
  released binaries were built with a toolchain that no longer receives
  security fixes. Build, test, lint, and release now use `stable`.

## [0.0.22] - 2026-07-28

### Changed

- **The command-line surface is branded too.** Until now the corral restyle
  only reached the full-screen dashboard, so `draft` and `draft --help` —
  the surface most people see most often — still printed plain, unstyled
  text. Help now opens with the gradient nib logo, wordmark, and tagline,
  with coral section headings and flag names over the reference text.
  Colour is dropped automatically when the output is piped, and
  `DRAFT_SHOW_LOGO=0` suppresses the logo. The logo, palette, and styles
  moved into a shared `internal/brand` package so the CLI and the dashboard
  cannot drift apart.

### Fixed

- **Help documents every flag.** `--frontmatter`, `--combine`,
  `DRAFT_EXTRACT_CONCURRENCY`, `DRAFT_SITE_*`, and `DRAFT_SHOW_LOGO` were
  missing from the reference text, and the OUTPUT section still described
  the pre-0.0.16 single-file layout instead of the day-folder set.

## [0.0.21] - 2026-07-28

### Added

- **`examples/dashboard`** runs the real full-screen TUI against an in-process
  demo engine — no model, no network, no source of your own. The article
  streams in word by word, so the queue, phase markers, live preview,
  progress bar, and focus timer animate exactly as they do on a real run.
  It is the quickest way to see the interface or check the layout at a
  given terminal size.

## [0.0.20] - 2026-07-28

### Fixed

- **The logo now shows on a standard terminal.** It was gated at 28 rows, so
  the common 80x24 window only ever got the one-line masthead. The layout is
  now budgeted line by line: the logo appears from 24 rows (compact form —
  tagline beside the wordmark), section rules and blank separators give way
  first on short terminals, and the focus timer and log are drawn only when
  the space left genuinely fits them. The running view no longer needs
  scrolling at any height from 20 rows up, with the queue and the full
  pipeline always visible. The nib artwork was redrawn with shoulders, a
  vent hole, a slit, and a tip.

## [0.0.19] - 2026-07-28

### Changed

- **The dashboard now speaks corral.** The TUI adopts the same design
  language as [corral](https://github.com/sebastienrousseau/corral): a
  red-gradient braille logo (a fountain-pen nib) with the `Draft.` wordmark
  and tagline, a single coral accent (`#F56B5E`) over a quiet gray ramp,
  flat divider-underlined sections instead of boxed panels, a coral-gradient
  progress bar, a `[key] action • …` shortcut bar, and the
  "Made with ❤️ in London, UK (vX)" footer. The logo collapses to a one-line
  masthead on short terminals and can be disabled with `DRAFT_SHOW_LOGO=0`;
  the activity line drops detail progressively so narrow terminals never
  wrap.

## [0.0.18] - 2026-07-28

### Added

- **Publisher identity via environment.** `DRAFT_SITE_*` variables
  (BASE_URL, CDN, NAME, SHORT_NAME, EMAIL, TWITTER, LOCATION,
  MEASUREMENT_ID, COPYRIGHT_FROM) override the frontmatter publisher
  identity for the CLI and pipeline alike; unset variables keep the
  defaults, and curated fields still always win.

### Changed

- **Library packages are now public.** `claims`, `config`, `engine`,
  `frontmatter`, `pipeline`, `prompt`, `rules`, and `validate` moved from
  `internal/` to the module root, so external projects can import them
  (`go get github.com/sebastienrousseau/draft`). Only the PDF extractor and
  the TUI remain internal. Import paths within this module changed
  accordingly; the CLI is unaffected.

## [0.0.17] - 2026-07-27

### Added

- **Complete example coverage.** Two new runnable, network-free demos:
  `examples/review` (surgical-edit enhancement end to end — body-only
  prompting, frontmatter re-attachment, and generated-set resync) and
  `examples/frontmatter` (metadata extraction, a custom `Site` publisher
  identity, the Split/Combine round trip, and the three regeneration rules,
  each demonstrated live). The pipeline demo now drafts from two sources,
  demonstrating `--merge`.

### Changed

- **README overhauled.** Grouped contents, a Library usage section with API
  synopses for the pipeline runner, grounding, frontmatter, and the `Engine`
  seam, an examples table, an honest when-not-to-use section, a documentation
  index, and the `frontmatter/` package added to the architecture tree.

## [0.0.16] - 2026-07-27

### Added

- **Modular frontmatter generation.** The new `internal/frontmatter` package
  extracts article metadata (title, deck or bold-line subtitle, TL;DR /
  executive-summary description, stopword-filtered keywords, frequency-scored
  category) and generates the standard YAML frontmatter schema. Every draft is
  now saved as a three-file set under the dated folder: `source/<stem>-body.md`
  (article only), `yaml/<stem>-frontmatter.yaml` (adjacent frontmatter), and
  `final/<stem>-final.md` (combined, ready to publish).
- **`--frontmatter <file>` (alias `--combine`)** regenerates the set from any
  article file. Regeneration is identity-preserving: the filename's
  `YYYY-MM-DD-slug` is canonical for dates and URLs, existing frontmatter
  fields always win over generated values (delete a field to have it
  regenerated from the body), and reprocessing an unchanged set is a
  byte-level no-op. Day-folder layouts route output to the sibling `source/`,
  `yaml/`, and `final/` directories.
- **`Site` publisher identity.** Generated frontmatter takes its author, URLs,
  handles, and analytics ID from a `Site` struct (`DefaultSite` by default),
  and the copyright end year follows the article date.

### Changed

- **`--review` is frontmatter-aware.** The draft's YAML block is set aside
  before prompting, so the model and the house rules only ever see the body;
  the frontmatter is re-attached on save, and reviewing one file of a
  generated set resyncs its siblings.
- **Draft sets can never desync.** The three output files are uniquified as a
  set — a leftover file in any one folder bumps all three names together.

## [0.0.15] - 2026-07-22

### Fixed

- **Style-calibration headings no longer leak into drafts.** The built-in style
  example showed a concrete heading outline ("What the result actually shows",
  "Why the mechanism matters", "Where it breaks") and `loadTemplates` fed the
  user's own template headings back as an outline — both of which a literal local
  model copied verbatim into unrelated articles. The built-in example now states
  the heading principle instead of showing copyable headings, and template style
  samples are stripped to prose only. Heading structure still comes from the
  output skeleton. Verified end to end: drafts now get specific, content-relevant
  section headings.

## [0.0.14] - 2026-07-22

### Fixed

- **Adverbial banned words are now caught and repaired too.** Building on the
  inflection handling in 0.0.13, the `-ly` form of a banned adjective
  ("seamlessly", "robustly", "profoundly") is now detected and rewritten to the
  replacement's adverb ("smoothly", "strongly", "deeply"), with correct spelling
  for `-ic` → `-ally`, `-le` → `-y`, and `-y` → `-ily` (so "bustling" repairs via
  "busy" → "busily"). This closes the residual noted in 0.0.13. The `vibrant`
  replacement moved to "vivid" so its adverb ("vividly") is clean.

## [0.0.13] - 2026-07-22

### Fixed

- **Banned words are now caught and repaired in their inflected forms.** Both the
  validator and the deterministic style repair matched only the base word, so
  "leverages", "leveraging", "utilizes", and "fostered" slipped through where
  "leverage" would have been caught. The banned vocabulary is now expanded to its
  common inflections (plural/third-person, past, gerund), and a matched form is
  replaced with the replacement inflected the same way — "leverages" becomes
  "uses", "leveraging" becomes "using" — using proper English spelling rules
  (silent-e, sibilant "-es"). Adverbial "-ly" forms remain a separate concern.

## [0.0.12] - 2026-07-22

### Changed

- **The metric-conversion guard now accepts a grounded expansion.** An
  abbreviation and its expansion — for example `bpb` and "bits per byte" — are the
  same metric, so using one in the draft when a claim uses the other is no longer
  flagged as a silent conversion. A switch to a genuinely different metric
  (perplexity where a claim says cross-entropy) is still caught, because those
  forms live in separate groups (`rules.MetricForms`). On a real metric-heavy
  paper this turned a repeated faithfulness failure into a clean first-attempt
  pass.

## [0.0.11] - 2026-07-22

### Changed

- **Hardened the article template against a literal local model.** Three
  refinements, none affecting a session provider's output:
  - **Every skeleton placeholder now self-heals.** A copied thesis label or bare
    `**...**` line is stripped, an unfilled `## ...` section heading is dropped
    (its body folds into the surrounding prose), and any ellipsis-only heading is
    caught by the validator — so a placeholder can neither ship nor fail a run.
  - **Style-calibration echo is removed.** A small model sometimes reproduces the
    tone example (or the user's own templates) as body text; any paragraph copied
    verbatim from the calibration block is now stripped from the draft.
  - **One source of truth for the house rules.** The writing and review prompts
    share a single `houseStyleRules` block instead of two near-duplicate lists,
    and the structural markers the validator checks for (`# `, `## `, the post-lead
    aside, the executive-summary label) live in `internal/rules`, with a test that
    keeps the skeleton in sync.

## [0.0.10] - 2026-07-21

### Changed

- **Parallel claim extraction on Ollama.** Sections are now mined two at a time
  against the local server (previously one at a time). On a single small GPU one
  request does not saturate the hardware, so with the server started at
  `OLLAMA_NUM_PARALLEL=2` two extractions run at ~1.8× the throughput of one; a
  server pinned to one slot simply queues the second, so it is safe either way.
  On a real 12-section paper this cut extraction from ~825s to ~645s. Capped at
  two for Ollama (override with `DRAFT_EXTRACT_CONCURRENCY`).

### Fixed

- **Opening-thesis placeholder no longer leaks or fails a run.** The skeleton's
  bold thesis was a concrete label ("Opening thesis paragraph.") that a literal
  model copied verbatim; on a dense paper this tripped the placeholder check and
  burned the whole retry budget. The label is gone from the skeleton, and
  post-processing now strips both a copied label (keeping any real thesis after
  it) and a bare unfilled `**...**` line. Combined with the above, a real
  12-section paper now drafts in ~817s (down from ~1330s) and passes on the first
  attempt.

## [0.0.9] - 2026-07-21

### Fixed

- **Skeleton placeholder no longer leaks into drafts.** The output skeleton used
  a concrete-looking heading ("First analytical section") that an obedient local
  model copied verbatim instead of replacing. The skeleton now uses a neutral
  placeholder, the writing prompt explicitly says to replace placeholders with
  specific headings, and the validator rejects any unfilled placeholder (a "..."
  heading or a leaked thesis marker) as a safety net. gemma now writes real,
  descriptive section headings.

## [0.0.8] - 2026-07-21

### Changed

- **Offline drafting is roughly 4× faster.** On a measured 8 GB machine a
  two-section source went from ~474s to ~116s end to end, with the draft passing
  the house rules on the first attempt instead of after retries. The gains come
  from three changes below; none reduce grounding.
- **Single Ollama model.** Writing now defaults to `gemma3:4b` (the model already
  used for extraction), so a memory-constrained server no longer swaps a second
  4B model in and out between phases. gemma also keeps to the word budget and does
  not leak planning text into the article, which `qwen3:4b` did. `qwen3:4b` is no
  longer used by default; the separate experimental `qwen` **session** provider is
  unaffected.

### Added

- **Claim-scaled length budget.** The target word count and the Ollama
  output-token cap are derived from the number of verified claims, so a thin
  ledger yields a short, fully-grounded draft rather than a padded one. A draft
  truncated at the cap is closed by trimming to its last complete sentence.
- **Deterministic style repair.** Banned cliché words and phrases are swapped for
  neutral, in-style equivalents in place (`internal/rules.StyleReplacements`),
  removing the most common reason an otherwise-clean local draft needed a full,
  slow regeneration.
- **`keep_alive` on Ollama requests** and a documented 8 GB server profile
  (flash attention + quantised KV cache) in the README — the single biggest
  offline speed-up, taking a cold run from minutes to under two.

## [0.0.7] - 2026-07-21

### Changed

- `cursor-agent` promoted to a verified (stable) provider after an end-to-end
  check. Its invocation now passes `--force` to clear the directory-trust prompt
  that otherwise blocks non-interactive runs. Stable providers are now claude,
  copilot, codex, grok, agy, and cursor-agent.

## [0.0.6] - 2026-07-21

### Changed

- Replaced the `gemini` provider with **`agy`** (Google Antigravity, the
  successor CLI), and promoted it to a verified (stable) provider after an
  end-to-end check. Stable providers are now claude, copilot, codex, grok, agy.

## [0.0.5] - 2026-07-21

### Added

- **Windows support.** The CI test matrix now covers Windows alongside Ubuntu
  and macOS, and releases ship Windows binaries (amd64 + arm64, as `.zip`). PDF,
  Markdown, and text sources work on all three platforms; DOCX remains
  macOS-only (`textutil`).

### Changed

- Made the `runTool` tests portable (use `go` instead of the `echo`/`false`
  shell builtins) so the suite passes on Windows.

## [0.0.4] - 2026-07-20

### Changed

- `grok` promoted to a verified (stable) provider after an end-to-end check
  (clean, grounded 1.1k-word draft). Stable providers are now claude, copilot,
  codex, and grok.

## [0.0.3] - 2026-07-19

### Added

- **Parallel claim extraction.** On a session provider, sections are mined
  concurrently (configurable via `DRAFT_EXTRACT_CONCURRENCY`, default 4); Ollama
  stays sequential. A failed worker retries down the fallback chain.
- **Live streaming preview.** The Claude backend now uses the `stream-json`
  event format, forwarding token deltas as they arrive instead of one jump.
- **Review mode.** `--review <draft.md>` enhances an existing draft with
  surgical find/replace edits grounded in the sources — validated for
  uniqueness and non-overlap, and re-checked against the house rules.
- **Cancellation** of in-flight work when the TUI quits or `--print` is
  interrupted (signal-aware context).
- **Experimental provider gating.** Only `claude`, `copilot`, and `codex` are
  verified end to end and used by auto mode; the rest need `--experimental`.

### Changed

- `codex` promoted to a verified (stable) provider after end-to-end checks.
- Removed the previously-dead surgical-edit code by wiring it into `--review`.
- DOCX extraction returns a clear "requires macOS" error off Darwin.

### Tooling

- GitHub Actions pinned to commit SHAs; ubuntu + macOS test matrix; full REUSE
  3.3 compliance with a `reuse` CI gate; GoReleaser release workflow attaching
  darwin/linux (amd64/arm64) binaries on tag push.

## [0.0.2] - 2026-07-19

### Added

- **Multi-provider session engines.** In `auto` mode `draft` now drives the
  first installed token-free coding-agent CLI — Claude, Codex, Gemini, Copilot,
  Cursor, Amp, Crush, Goose, Grok, or Qwen — through its own logged-in session.
  Force one with `--engine <name>`; override the model with `--model`.
- **Engine fallback chain.** A failed session call advances along the chain and
  finally to Ollama, so a queue works online, offline, or across a change in
  connectivity — each job re-selects its engine independently.
- **`--keep-artifacts`.** A successful run now leaves only the finished article
  in the dated folder; the scratch claim ledger is removed unless this flag is
  set.
- **`--print` headless mode**, `examples/` with three runnable, network-free
  demos, benchmarks for the hot paths, and godoc examples.
- Test coverage raised to **≥95%** of app/library statements, including the
  session backends (faked via the `TestHelperProcess` pattern).
- **Cancellation.** A signal-aware context is threaded through the run; quitting
  the TUI or Ctrl+C in `--print` terminates any in-flight provider subprocess or
  Ollama request instead of orphaning it.
- **Experimental provider gating.** Only `claude` and `copilot` are verified end
  to end and used by auto mode; the rest are marked experimental and require
  `--experimental` (or `--engine <name>`), so the breadth claim stays honest.

### Changed

- DOCX extraction now returns a clear "requires macOS" error off Darwin instead
  of a confusing missing-command failure.
- Removed the unwired surgical-review prompt (`prompt.Review`) and its dead
  helpers rather than shipping unused code.
- Documented the agent auto-approve trust surface in the README security notes.

### Fixed

- **Header no longer bleeds.** The status line is fitted to the terminal width
  and hard-clipped, dropping the tagline then the model/word-range as space
  shrinks.
- **Logo** renders the Kenaz rune with a bright wordmark.
- **Progress bar** shows an explicit percentage.
- **`.golangci.yml`** migrated to the v2 schema; `make lint` runs clean.
- Truncation check decodes the final rune, so a draft ending in a smart quote or
  ellipsis is no longer flagged as truncated.

## [0.0.1] - 2026-07-19

### Added

- Initial release: a Bubble Tea CLI that turns research PDFs into grounded,
  body-only Markdown drafts, writing with Claude via the active CLI session when
  online and a local Ollama model when offline, grounded by a verified claim
  ledger.

[0.0.33]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.33
[0.0.32]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.32
[0.0.31]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.31
[0.0.30]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.30
[0.0.29]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.29
[0.0.28]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.28
[0.0.27]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.27
[0.0.26]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.26
[0.0.25]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.25
[0.0.24]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.24
[0.0.23]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.23
[0.0.22]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.22
[0.0.21]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.21
[0.0.20]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.20
[0.0.19]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.19
[0.0.18]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.18
[0.0.17]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.17
[0.0.16]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.16
[0.0.15]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.15
[0.0.14]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.14
[0.0.13]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.13
[0.0.12]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.12
[0.0.11]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.11
[0.0.10]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.10
[0.0.9]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.9
[0.0.8]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.8
[0.0.7]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.7
[0.0.6]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.6
[0.0.5]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.5
[0.0.4]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.4
[0.0.3]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.3
[0.0.2]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.2
[0.0.1]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.1
