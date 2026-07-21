# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/), and versions use a `0.0.x`
series until `0.0.999`.

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

[0.0.9]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.9
[0.0.8]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.8
[0.0.7]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.7
[0.0.6]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.6
[0.0.5]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.5
[0.0.4]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.4
[0.0.3]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.3
[0.0.2]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.2
[0.0.1]: https://github.com/sebastienrousseau/draft/releases/tag/v0.0.1
