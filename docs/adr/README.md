# Architecture decision records

One file per decision that will be questioned later, in the order they were
made. Each records the context at the time, what was chosen, and what the
choice costs — a record whose "consequences" section is all upside is a
advertisement, not a decision record.

Format: [MADR](https://adr.github.io/madr/)-flavoured, deliberately short.

| #                                                  | Decision                                                                 | Status   |
| -------------------------------------------------- | ------------------------------------------------------------------------ | -------- |
| [0001](0001-plain-text-extraction.md)              | Extract with `pdftotext`, not a document-understanding toolkit           | Accepted |
| [0002](0002-quote-verified-claim-ledger.md)        | Ground by verifying quotes before writing, not by checking after         | Accepted |
| [0003](0003-session-cli-backends.md)               | Drive already-authenticated agent CLIs instead of handling API keys      | Accepted |
| [0004](0004-no-tools-granted-to-providers.md)      | Grant provider subprocesses no tools, and run them in an empty directory | Accepted |
| [0005](0005-content-addressed-extraction-cache.md) | Address the extraction cache by content, not by date                     | Accepted |

## Writing a new one

Copy [`template.md`](template.md), take the next number, add a row above. A
decision that only ever had one plausible option does not need a record; a
decision someone will later call obviously wrong does.
