# 0008. Attribute sentences to claims; emit a C2PA manifest definition

- **Status:** Accepted
- **Date:** 2026-09

## Context

The verified claim ledger proves the article is grounded, as a whole. It
cannot say which claim backs sentence 14, and nothing binds the file a
reader holds to the ledger it was verified against. The 2026 direction in
grounding research is traceability from output span to evidence; content
provenance is standardising on C2PA, whose 2.3 specification covers text.
A product whose promise is authenticity should produce an artefact a reader
can check, not a log line.

## Decision

A `provenance` package with two outputs, written by `save` beside every set.

Attribution assigns each claim a stable identifier derived from its
normalised quote, splits the body into prose sentences with byte offsets,
and ties a sentence to a claim when they share a figure and a content word,
or enough content words that the overlap is not chance. It is computed from
the text alone: deterministic, model-free, and re-runnable by anyone with the
body and the ledger.

The manifest is a C2PA manifest definition in the shape `c2patool` reads:
generator, a `c2pa.created` action naming draft as the software agent, the
sources as ingredients, and a `com.draftlib.grounding` assertion carrying
the digests, the run's identity and the claim identifiers. draft does not
sign it.

## Alternatives considered

- **Ask the writer to cite claim identifiers inline.** Would change the
  writing prompt and the article's structure, put citation-shaped tokens in
  prose the house style forbids, and make attribution only as honest as the
  model's citations. Post-hoc attribution can be checked; a citation can
  only be trusted.
- **A learned verifier for attribution.** Better recall on paraphrase, at the
  cost of a model dependency in the one part of the pipeline that is
  deterministic today. Left open; the lexical map is a floor.
- **Embed and sign a real C2PA manifest.** Requires a signing identity and a
  JUMBF implementation; both belong to the publisher. A definition the
  reference tool consumes is the boundary draft can honestly own.
- **Put the attribution in the frontmatter.** Hundreds of lines in a file the
  regeneration contract promises to keep stable.

## Consequences

- A fourth directory in the set. `--frontmatter` does not regenerate it,
  because an edited body is exactly what invalidates it.
- Attribution is evidence, not proof, and its field names say so
  (`evidence`, `ungrounded_numbers`). A low attributed count on a long
  article is worth a look, not a failure.
- The `--json` record gains a `provenance` object; the schema number does not
  change, because nothing existing moved.
