# 0005. Address the extraction cache by content, not by date

- **Status:** Accepted
- **Date:** 2026-09

## Context

Claim extraction is 80–95% of a run's wall clock. The only reuse was
`--resume`, whose ledger is named for today's date and the first source's
filename and written into the dated output folder.

That key is wrong in three ordinary situations: redrafting the same paper
tomorrow finds nothing, renaming the file finds nothing, and a `--merge` that
overlaps an earlier run re-extracts the shared sections. Each re-pays the
dominant cost in full.

## Decision

Store each section's raw extraction under
`sha256(prompt version, engine, model, section body)`, in the user's cache
directory. The prompt version is a hash of the extraction template with an
empty untrusted block, so it is stable across calls despite the per-call nonce
while still invalidating every entry when the instructions change.

## Alternatives considered

- **Keep the date-scoped ledger.** Correct but reuses almost nothing.
- **Key on the file path.** Breaks on rename and on two copies of one paper.
- **Key on the file hash.** Better, but still re-extracts every section of a
  merged job when one source changed. Sections are the unit of work.
- **Cache the parsed ledger rather than the raw output.** Would make the cache
  authoritative over grounding, which is exactly what the next point forbids.

## Consequences

- A redraft, a rename and an overlapping merge all become free. Measured on a
  one-section source: the extract phase fell from 15,108 ms to 1 ms, with an
  identical ledger digest.
- **A cached entry is never trusted on its own account.** The raw text is
  re-parsed and re-verified against the freshly read section, so a stale or
  tampered entry can only ever yield *fewer* verified claims, never an
  ungrounded one — the same property that makes `--resume` safe. A test replays
  an entry against a section that no longer supports it and asserts the drop.
- Cache state now exists on disk. Entries expire after 30 days, a corrupt one
  is removed rather than becoming a permanent miss, `--no-cache` skips it, and
  `--clear-cache` refuses to touch anything that is not a cache shard.
- Writes go through `internal/atomicfile`, because parallel extraction workers
  would otherwise let a reader see a half-written entry and delete it as
  corrupt.
