# 0001. Extract with `pdftotext`, not a document-understanding toolkit

- **Status:** Accepted
- **Date:** 2026-07

## Context

Every claim must quote its source verbatim, so extraction quality is not a
cosmetic concern: a garbled span cannot be matched and its claim is dropped.

Measured on this machine, Poppler processes a 62-page chapter at roughly
**580 pages/second**. Published figures for the alternatives, on comparable or
better hardware: Marker (fast, no OCR) 23.7 pages/s, Docling 2.2–2.5 pages/s on
an M3 Max, Unstructured 0.24 pages/s.

The quality gap runs the other way. On olmOCR-Bench a PDFium-class plain-text
extractor scores **20.4%** against Docling's 50.3% and Marker's 76.0%, because
that benchmark rewards table structure, LaTeX maths and scanned pages.

## Decision

Shell out to `pdftotext` in reading order (not `-layout`), and accept that
tables, formulae and scans are out of scope.

## Alternatives considered

- **Docling / Marker / MinerU.** Two to three orders of magnitude slower, and
  each brings a Python and model-weight dependency that would end the "one
  static binary, no GPU, works offline" property the tool is built around.
- **A cgo PDF library.** Removes the runtime dependency but reintroduces cgo,
  which costs cross-compilation and the static Linux binary.
- **`pdftotext -layout`.** Rejected on evidence: preserving visual layout
  splices the two columns of a paper onto shared lines, so sentences merge with
  unrelated text and a quote can never match. Spliced lines across the corpus
  went from 158 and 59 on two papers to zero once reading order was used.

## Consequences

- The deterministic path is ~0.25 s, so essentially all of a run is model
  latency. That ratio is the product.
- **Recall, not precision, is what suffers.** A claim drawn from a table cannot
  be quote-matched, so it is dropped. The draft stays honest and becomes
  thinner, and the run reports the loss only as a count — a user is likely to
  read it as the model being lazy.
- Scanned PDFs are refused with an OCR suggestion rather than silently
  producing nothing.
- A pluggable extractor interface, letting a user route table-heavy sources
  through Docling while keeping this as the default, is the open follow-up.
