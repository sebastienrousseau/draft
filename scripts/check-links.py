#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau
# SPDX-License-Identifier: MIT OR Apache-2.0
"""Check every intra-repository Markdown link, including heading anchors.

External URLs are deliberately not checked: they break for reasons that have
nothing to do with a change, and a gate that fails on someone else's outage is
one people learn to ignore.

Run with `make docs-lint`, which is the same command CI runs.
"""
import glob
import os
import re
import sys
import urllib.parse

LINK = re.compile(r"\[[^\]]*\]\(([^)]+)\)")
ANCHOR_TAG = re.compile(r'<a\s+id="([^"]+)"')
HEADING = re.compile(r"^(#{1,6})\s+(.*)$", re.M)


def slug(text: str) -> str:
    """Reproduce GitHub's heading slugger.

    Note it replaces each space individually and does NOT collapse runs, so
    "A — B" becomes "a--b". Collapsing them silently reports good links as
    broken.
    """
    t = re.sub(r"<[^>]+>", "", text)
    t = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", t)
    t = t.lower().strip()
    t = re.sub(r"[^\w\s-]", "", t)
    return t.replace(" ", "-")


def anchors(path: str) -> set[str]:
    text = open(path, encoding="utf-8").read()
    found = {slug(h[1]) for h in HEADING.findall(text)}
    return found | set(ANCHOR_TAG.findall(text))


def main() -> int:
    bad = []
    for path in sorted(glob.glob("**/*.md", recursive=True)):
        if path.startswith(("node_modules", ".git")):
            continue
        for raw in LINK.findall(open(path, encoding="utf-8").read()):
            target = raw.split(" ")[0].strip("<>")
            if target.startswith(("http://", "https://", "mailto:", "#!")):
                continue
            frag = ""
            if "#" in target:
                target, frag = target.split("#", 1)
            frag = urllib.parse.unquote(frag)
            if target == "":
                dest = path
            else:
                dest = os.path.normpath(
                    os.path.join(os.path.dirname(path), urllib.parse.unquote(target))
                )
                if not os.path.exists(dest):
                    bad.append(f"{path}: missing file -> {raw}")
                    continue
            if frag and dest.endswith(".md") and frag not in anchors(dest):
                bad.append(f"{path}: missing anchor -> {raw}")

    for b in bad:
        print(f"broken link: {b}", file=sys.stderr)
    print(f"checked intra-repo links; {len(bad)} broken")
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main())
