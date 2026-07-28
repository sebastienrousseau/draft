# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/), and versions use a `0.0.x`
series until `0.0.999`.

## [0.0.24] - 2026-07-28

### Added

- **Signed releases with SBOMs and build provenance.** Every archive now
  ships a CycloneDX SBOM, `checksums.txt` is signed keylessly with Sigstore
  cosign (certificate and signature published alongside), and GitHub build
  provenance is attested for each artefact. Consumers can verify what they
  downloaded and where it was built.
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
