#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau
# SPDX-License-Identifier: MIT OR Apache-2.0
"""Align Markdown table pipes, the way markdownlint's MD060 wants them.

Padding is by *display* width, not character count: an emoji is one character
and two columns, so a character-counted table looks aligned to Python and
ragged to everything else.
"""
import glob
import sys
import unicodedata


def width(s: str) -> int:
    return sum(
        0 if unicodedata.combining(c) else 2 if unicodedata.east_asian_width(c) in ("W", "F") else 1
        for c in s
    )


def pad(s: str, w: int) -> str:
    return s + " " * max(0, w - width(s))


def is_row(line: str) -> bool:
    stripped = line.strip()
    return stripped.startswith("|") and stripped.endswith("|") and len(stripped) > 1


def realign(block: list[str]) -> list[str]:
    rows = [[c.strip() for c in l.strip().strip("|").split("|")] for l in block]
    n = max(len(r) for r in rows)
    rows = [r + [""] * (n - len(r)) for r in rows]

    sep = next(
        (i for i, r in enumerate(rows) if all(c and set(c) <= set("-: ") for c in r)), None
    )
    if sep is None:
        return block

    w = [0] * n
    for i, r in enumerate(rows):
        if i == sep:
            continue
        for j, c in enumerate(r):
            w[j] = max(w[j], width(c))

    out = []
    for i, r in enumerate(rows):
        if i == sep:
            cells = []
            for j in range(n):
                marker = rows[sep][j]
                left, right = marker.startswith(":"), marker.endswith(":")
                body = "-" * max(1, w[j] - int(left) - int(right))
                cells.append((":" if left else "") + body + (":" if right else ""))
            out.append("| " + " | ".join(cells) + " |")
        else:
            out.append("| " + " | ".join(pad(r[j], w[j]) for j in range(n)) + " |")
    return out


def main() -> int:
    changed = []
    for path in sorted(glob.glob("**/*.md", recursive=True)):
        if path.startswith(("node_modules", ".git")):
            continue
        lines = open(path, encoding="utf-8").read().split("\n")
        out, i, fenced = [], 0, False
        while i < len(lines):
            line = lines[i]
            if line.lstrip().startswith(("```", "~~~")):
                fenced = not fenced
                out.append(line)
                i += 1
                continue
            if not fenced and is_row(line):
                j = i
                while j < len(lines) and is_row(lines[j]):
                    j += 1
                out += realign(lines[i:j])
                i = j
                continue
            out.append(line)
            i += 1
        new = "\n".join(out)
        if new != "\n".join(lines):
            open(path, "w", encoding="utf-8").write(new)
            changed.append(path)
    for p in changed:
        print(f"aligned tables in {p}")
    print(f"{len(changed)} file(s) changed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
