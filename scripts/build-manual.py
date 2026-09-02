#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau
# SPDX-License-Identifier: MIT OR Apache-2.0
"""Assemble the MkDocs source tree from the Markdown already in the repository.

The manual is a *rendering* of these files, never a second copy of them: the
chapters are the repository's own documents, copied into place with their
directory structure preserved so every relative link keeps resolving without
rewriting. Nothing here is authored; if a chapter is wrong, the file it came
from is wrong.

Root documents land at the top level and docs/ keeps its subdirectory, which is
what makes links like [ARCHITECTURE](docs/ARCHITECTURE.md) and ../README.md
work identically in the repository and on the site.
"""
import glob
import os
import re
import shutil
import sys

# A Markdown link whose target is the repository README, at any depth. ADR
# links to a sibling README (docs/adr/README.md) are deliberately not matched:
# those really are their own page.
README_LINK = re.compile(r"\]\((\.\./)*README\.md(#[^)]*)?\)")

# A link into the source tree. Those resolve on GitHub and cannot resolve on a
# documentation site, so they are pointed at the repository instead of being
# left to 404.
REPO = "https://github.com/sebastienrousseau/draft/tree/main"
SOURCE_LINK = re.compile(r"\]\((examples|internal|cmd|pipeline|engine|claims|validate|rules|prompt|frontmatter|config|scripts)(/[^)#]*)?\)")

# A root file that is not documentation: it exists in the repository but has no
# page on the site.
ROOT_FILE_LINK = re.compile(
    r"\]\((LICENSE-MIT|LICENSE-APACHE|go\.mod|go\.sum|Makefile|GNUmakefile|REUSE\.toml|mkdocs\.yml|\.goreleaser\.yaml)\)"
)

# A link to a directory. GitHub renders a listing; a documentation site has no
# such page, so these are pointed at the repository too.
DIR_LINK = re.compile(r"\]\(((?:\.\./)*[A-Za-z0-9_./-]+/)\)")


def to_repo_url(m: re.Match) -> str:
    return f"]({REPO}/{m.group(1)}{m.group(2) or ''})"


def dir_to_repo_url(m: re.Match, page: str) -> str:
    """Resolve a directory link against the page holding it, then point at the
    repository. A relative ../ in docs/ADR means something different than the
    same text in a root document."""
    target = os.path.normpath(os.path.join(os.path.dirname(page), m.group(1)))
    return f"]({REPO}/{target.replace(os.sep, '/')}/)"

OUT = ".gen/manual"

# Root documents, and the name each takes in the manual. README becomes the
# landing page; MkDocs needs it called index.md.
ROOT_PAGES = {
    "README.md": "index.md",
    "DEVELOPMENT.md": "DEVELOPMENT.md",
    "CONTRIBUTING.md": "CONTRIBUTING.md",
    "SECURITY.md": "SECURITY.md",
    "SUPPORT.md": "SUPPORT.md",
    "GOVERNANCE.md": "GOVERNANCE.md",
    "CODE_OF_CONDUCT.md": "CODE_OF_CONDUCT.md",
    "CHANGELOG.md": "CHANGELOG.md",
}


def main() -> int:
    if os.path.exists(OUT):
        shutil.rmtree(OUT)
    os.makedirs(OUT)

    copied = 0
    for src, dst in ROOT_PAGES.items():
        if not os.path.exists(src):
            continue
        shutil.copy2(src, os.path.join(OUT, dst))
        copied += 1

    for src in glob.glob("docs/**/*.md", recursive=True):
        dst = os.path.join(OUT, src)
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        shutil.copy2(src, dst)
        copied += 1

    # MkDocs treats README.md as an alias for index.md, so a page cannot be
    # both. The landing page is the README, which means every link *to* the
    # README has to point at index.md instead. This is the only content
    # transformation in the assembly, and it is mechanical.
    retargeted = 0
    for path in glob.glob(f"{OUT}/**/*.md", recursive=True):
        text = open(path, encoding="utf-8").read()
        fixed = README_LINK.sub(lambda m: m.group(0).replace("README.md", "index.md"), text)
        fixed = SOURCE_LINK.sub(to_repo_url, fixed)
        fixed = ROOT_FILE_LINK.sub(lambda m: f"]({REPO}/{m.group(1)})", fixed)
        page = os.path.relpath(path, OUT)
        fixed = DIR_LINK.sub(lambda m: dir_to_repo_url(m, page), fixed)
        if fixed != text:
            open(path, "w", encoding="utf-8").write(fixed)
            retargeted += 1

    print(f"assembled {copied} page(s) into {OUT}; retargeted README links in {retargeted}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
