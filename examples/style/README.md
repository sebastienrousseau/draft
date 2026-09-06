<!-- SPDX-FileCopyrightText: 2026 Sebastien Rousseau -->
<!-- SPDX-License-Identifier: MIT OR Apache-2.0 -->

# Custom house style

`draft --style <file.json>` (or `DRAFT_STYLE=<file.json>`) replaces draft's
built-in editorial policy with your own. Only the fields you set change; the
rest keep their defaults, and the structural rules — an H1, a lead aside, an
executive summary, section headings — are fixed, because they are the shape of
a grounded article rather than a matter of taste.

| Field | Meaning |
| --- | --- |
| `min_words`, `max_words` | The word band a finished article must fall in. |
| `english` | The language-variant instruction to the writer, e.g. `"British English"`. Empty means no instruction. |
| `banned_words` | Replaces the built-in banned-word list outright. |
| `banned_phrases` | Replaces the built-in banned-phrase list outright. |
| `also_banned_words` | Adds to the list in force (the default, or your replacement). |
| `also_banned_phrases` | Adds to the list in force. |

An unknown field or an impossible word band is a clean error that falls back to
the default style, never a failed run. See [`house-style.json`](house-style.json).
