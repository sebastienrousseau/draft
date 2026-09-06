# 0007. Make the document reader pluggable, with plain text the default

- **Status:** Accepted
- **Date:** 2026-09

## Context

[ADR 0001](0001-plain-text-extraction.md) chose `pdftotext` for speed and
portability, and the audit priced the trade: on olmOCR-Bench a PDFium-class
plain-text extractor scores 20.4% against Marker's 76.0% and Docling's
50.3%, the losses being tables, formulas, reading order and headers. For a
prose paper that is invisible; for a paper whose results live in a table it
is most of the evidence.

## Decision

`internal/pdf.ExtractWith` takes a reader name. `pdftotext` remains the
default and the fast path; `docling` shells out to the Docling CLI for PDF and
DOCX and reads its Markdown. Markdown and text sources never go through a
reader. The choice is per run (`--reader`, `DRAFT_READER`), validated at
start-up, reported by `--dry-run`, and probed by `--doctor`.

## Alternatives considered

- **Docling by default.** A 62-page paper that took 110 ms would take
  minutes, and every user would need a Python installation and a model
  download before their first draft. The default stays the tool that runs
  everywhere in a tenth of a second.
- **Marker.** Faster on a GPU and stronger on maths, but GPU-first, and its
  CPU story is weaker than Docling's. Adding it later is one more case in
  the same switch.
- **A Go PDF library.** Nothing in Go reproduces a layout model; it would
  trade one plain-text extractor for another.

## Consequences

- Tables reach the ledger as Markdown tables, so a claim can quote a cell.
- DOCX works off macOS when Docling is chosen.
- The extraction cache keys on section text, so the two readers never serve
  each other's sections.
- Measured on a 1.9 MB arXiv paper (2412.20138): Docling took 155 s against
  under a second, returned 14,374 raw words against pdftotext's 17,191, and
  kept 10 tables and 68 headings that plain text flattened. The shorter text
  is consistent with running headers and page furniture being dropped; the
  difference has not been audited line by line, and a table-heavy corpus to
  measure recall on is still the open item from the September audit.
- Docling's own quirks reach the sections: on one web-page PDF it split
  ligatures ("Swi ft"). Grounding is unaffected, because quotes are verified
  against the same text, but the writer sees what the reader saw.
