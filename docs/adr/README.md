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
| [0006](0006-agent-client-protocol-transport.md)    | Add an Agent Client Protocol transport beside the one-shot CLIs          | Accepted |
| [0007](0007-pluggable-document-reader.md)          | Make the document reader pluggable, with plain text the default          | Accepted |
| [0008](0008-sentence-attribution-c2pa.md)          | Attribute sentences to claims; emit a C2PA manifest definition           | Accepted |
| [0009](0009-optional-semantic-second-gate.md)      | Add an opt-in semantic second gate over the verified claims              | Accepted |
| [0010](0010-signed-c2pa-credential.md)             | Sign the C2PA manifest, and emit a portable verification record          | Accepted |
| [0011](0011-direct-api-escape-hatch.md)            | Add a direct-API escape hatch beside the keyless default                 | Accepted |

## Writing a new one

Copy [`template.md`](template.md), take the next number, add a row above. A
decision that only ever had one plausible option does not need a record; a
decision someone will later call obviously wrong does.
