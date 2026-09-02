#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau
# SPDX-License-Identifier: MIT OR Apache-2.0
"""Check that the version numbers scattered around the repository agree.

A version in a README snippet, a citation file or a changelog heading is a
claim, and claims drift silently. This checks the ones that can be checked
mechanically:

  1. every git tag has a changelog entry;
  2. CITATION.cff names the newest tag;
  3. any changelog heading that claims a release with no tag behind it.

(3) is reported but does not fail: cutting a release, or relabelling a section
that was never released, is a maintainer decision and not something CI should
force. It is printed loudly because it means the changelog is telling users
about a version they cannot install.
"""
import re
import subprocess
import sys

CHANGELOG = "CHANGELOG.md"
CITATION = "CITATION.cff"
RELEASE_HEADING = re.compile(r"^## \[(\d+\.\d+\.\d+)\]", re.M)


def tags() -> list[str]:
    out = subprocess.run(
        ["git", "tag", "--sort=-v:refname"], capture_output=True, text=True, check=True
    ).stdout
    return [t[1:] if t.startswith("v") else t for t in out.split() if t.strip()]


def main() -> int:
    problems: list[str] = []
    notes: list[str] = []

    changelog_versions = RELEASE_HEADING.findall(open(CHANGELOG, encoding="utf-8").read())
    tagged = tags()
    if not tagged:
        print("no tags yet; nothing to verify")
        return 0

    newest = tagged[0]

    # 1. Every tag must be documented.
    for t in tagged:
        if t not in changelog_versions:
            problems.append(f"tag v{t} has no '## [{t}]' section in {CHANGELOG}")

    # 2. The citation must name the newest release.
    citation = open(CITATION, encoding="utf-8").read()
    m = re.search(r"^version:\s*(\S+)\s*$", citation, re.M)
    if not m:
        problems.append(f"{CITATION} has no version field")
    elif m.group(1) != newest:
        problems.append(
            f"{CITATION} says version {m.group(1)}, but the newest tag is v{newest}"
        )

    # 3. A documented release with no tag is a version users cannot install.
    for v in changelog_versions:
        if v not in tagged:
            notes.append(
                f"{CHANGELOG} documents {v} as released, but there is no v{v} tag "
                f"and no release. Either tag it, or move that section under "
                f"[Unreleased]."
            )

    for n in notes:
        print(f"note: {n}")
    for p in problems:
        print(f"error: {p}", file=sys.stderr)

    if problems:
        return 1
    print(f"versions agree; newest release is v{newest}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
