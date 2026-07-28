#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau
# SPDX-License-Identifier: MIT OR Apache-2.0
"""Generate two-column.pdf, the fixture for column and truncation handling.

The fixture deliberately mimics the shape of a real conference paper that
broke extraction in production:

  * page 1 carries a contents list whose "References  4" line sits far above
    the real bibliography, so a first-match truncation rule cuts the paper
    off in its front matter;
  * pages 1-2 set body prose in two columns, so an extractor that preserves
    physical layout splices the left and right columns onto shared lines;
  * page 3 holds the real References section, which should be dropped.

Regenerate with:  python3 make_fixture.py
"""

import zlib

LEFT_X, RIGHT_X, TOP_Y, LEADING = 56, 320, 720, 14
FONT_SIZE = 9

# Lines are kept short so the two columns cannot overlap on the page: at 9pt
# Helvetica a 46-character line is about 207pt wide, leaving a clear gutter
# between the left column at x=56 and the right column at x=320.
LEFT_COL = [
    "Sparse routing reduces the compute a dense",
    "model spends on tokens that are trivially",
    "predictable. We evaluate Router-S against a",
    "dense baseline on an identical token budget.",
    "The headline result is that Router-S reaches",
    "a validation loss of 3.41 while consuming",
    "five times fewer FLOPs than the dense",
    "baseline it replaces. That saving is not",
    "uniform across the corpus; it concentrates",
    "on passages the router marks as low entropy.",
]

RIGHT_COL = [
    "Our evaluation harness holds the data order",
    "fixed across runs so that differences in loss",
    "cannot be attributed to shuffling. Each",
    "configuration is trained three times and we",
    "report the median, since seed variance",
    "exceeds the gap we are measuring here.",
    "Prior work on conditional computation",
    "reports similar savings but measures them",
    "on synthetic corpora. We show that the",
    "router learns the skipping policy unaided.",
]


def esc(s: str) -> str:
    return s.replace("\\", r"\\").replace("(", r"\(").replace(")", r"\)")


def text_block(x, y, lines, size=FONT_SIZE):
    out = [f"BT /F1 {size} Tf {x} {y} Td {LEADING} TL"]
    for i, line in enumerate(lines):
        out.append(f"({esc(line)}) Tj" if i == 0 else f"T* ({esc(line)}) Tj")
    out.append("ET")
    return "\n".join(out)


def page1():
    parts = [
        text_block(LEFT_X, 750, ["Router-S: Conditional Compute for Language Models"], 14),
        # A contents list — note "References" appears here, long before the
        # real bibliography on the final page.
        text_block(LEFT_X, TOP_Y, [
            "Contents",
            "1  Introduction                                       1",
            "2  Method                                             2",
            "3  Results                                            3",
            "   References                                         4",
        ]),
        text_block(LEFT_X, 620, ["Abstract"], 11),
        text_block(LEFT_X, 600, LEFT_COL[:5]),
        text_block(RIGHT_X, 600, RIGHT_COL[:5]),
        text_block(LEFT_X, 500, ["Introduction"], 11),
        text_block(LEFT_X, 480, LEFT_COL[5:]),
        text_block(RIGHT_X, 480, RIGHT_COL[5:]),
    ]
    return "\n".join(parts)


def page2():
    return "\n".join([
        text_block(LEFT_X, 750, ["Method"], 11),
        text_block(LEFT_X, TOP_Y, LEFT_COL),
        text_block(RIGHT_X, TOP_Y, RIGHT_COL),
        text_block(LEFT_X, 560, ["Results"], 11),
        text_block(LEFT_X, 540, LEFT_COL[:6]),
        text_block(RIGHT_X, 540, RIGHT_COL[:6]),
    ])


def page3():
    return "\n".join([
        text_block(LEFT_X, 750, ["References"], 11),
        text_block(LEFT_X, TOP_Y, [
            "[1] A. Researcher. Conditional computation. 2024.",
            "[2] B. Author. Sparse routing at scale. 2025.",
            "[3] C. Scientist. Entropy and capacity. 2026.",
        ]),
    ])


def build():
    objs, streams = [], [page1(), page2(), page3()]
    n_pages = len(streams)
    kids = " ".join(f"{4 + i} 0 R" for i in range(n_pages))

    objs.append("<< /Type /Catalog /Pages 2 0 R >>")
    objs.append(f"<< /Type /Pages /Kids [{kids}] /Count {n_pages} >>")
    objs.append("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
    for i in range(n_pages):
        objs.append(
            f"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "
            f"/Resources << /Font << /F1 3 0 R >> >> /Contents {4 + n_pages + i} 0 R >>"
        )

    body = b""
    offsets = []
    out = b"%PDF-1.4\n"

    def emit(num, payload: bytes):
        nonlocal out
        offsets.append(len(out))
        out += f"{num} 0 obj\n".encode() + payload + b"\nendobj\n"

    for i, o in enumerate(objs, start=1):
        emit(i, o.encode())
    for i, s in enumerate(streams, start=4 + n_pages):
        data = zlib.compress(s.encode())
        emit(i, f"<< /Length {len(data)} /Filter /FlateDecode >>\nstream\n".encode()
             + data + b"\nendstream")

    total = len(objs) + len(streams)
    xref_at = len(out)
    out += f"xref\n0 {total + 1}\n0000000000 65535 f \n".encode()
    for off in offsets:
        out += f"{off:010d} 00000 n \n".encode()
    out += (f"trailer\n<< /Size {total + 1} /Root 1 0 R >>\n"
            f"startxref\n{xref_at}\n%%EOF\n").encode()
    return out


def build_scanned():
    """A page with graphics but no text layer, as a scanner would produce."""
    objs = [
        "<< /Type /Catalog /Pages 2 0 R >>",
        "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>",
    ]
    stream = "0.5 g\n56 600 500 120 re f\n56 400 500 160 re f\n"
    out = b"%PDF-1.4\n"
    offsets = []

    def emit(num, payload: bytes):
        nonlocal out
        offsets.append(len(out))
        out += f"{num} 0 obj\n".encode() + payload + b"\nendobj\n"

    for i, o in enumerate(objs, start=1):
        emit(i, o.encode())
    data = zlib.compress(stream.encode())
    emit(4, f"<< /Length {len(data)} /Filter /FlateDecode >>\nstream\n".encode()
         + data + b"\nendstream")

    xref_at = len(out)
    out += f"xref\n0 5\n0000000000 65535 f \n".encode()
    for off in offsets:
        out += f"{off:010d} 00000 n \n".encode()
    out += (b"trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n"
            + str(xref_at).encode() + b"\n%%EOF\n")
    return out


if __name__ == "__main__":
    with open("two-column.pdf", "wb") as fh:
        fh.write(build())
    print("wrote two-column.pdf")
    with open("no-text-layer.pdf", "wb") as fh:
        fh.write(build_scanned())
    print("wrote no-text-layer.pdf")
