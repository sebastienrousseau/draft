// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pdf

import (
	"regexp"
	"strings"
)

// texBody matches the content of a LaTeX document body, so the preamble
// (documentclass, package imports, macro definitions) is dropped before
// sectioning. The preamble carries no claimable prose, only configuration, and
// left in it becomes a junk first section the miner spends a call rejecting.
var texBody = regexp.MustCompile(`(?s)\\begin\{document\}(.*?)\\end\{document\}`)

// deTeX turns LaTeX source into the plain text a claim can be grounded in.
//
// The motivation for accepting .tex at all is that a formula is exact text in
// the source, where pdftotext scrambles it and even a layout reader can only
// approximate it. So the transform is deliberately light: it removes the noise
// that is never quotable — comments and the preamble — and leaves the body,
// maths included, otherwise verbatim. It is NOT a full LaTeX-to-text
// conversion: macro expansion, environment rendering and citation resolution
// are out of scope here and belong to a heavier reader (Docling, pandoc). The
// text this returns is exactly what the extractor quotes against, so whatever
// it leaves in place a SOURCE_QUOTE can still match byte for byte.
func deTeX(src string) string {
	src = stripTeXComments(src)
	if m := texBody.FindStringSubmatch(src); m != nil {
		src = m[1]
	}
	return src
}

// stripTeXComments removes LaTeX line comments: an unescaped % to the end of
// the line, and the newline with it, exactly as TeX does. A \% is a literal
// percent sign and is kept. Because a comment can never be quoted from the
// rendered document, dropping it keeps the source text aligned with what a
// reader of the paper would see.
func stripTeXComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for _, line := range strings.SplitAfter(src, "\n") {
		cut := -1
		for i := 0; i < len(line); i++ {
			if line[i] != '%' {
				continue
			}
			// A percent escaped by a backslash that is not itself escaped is a
			// literal, so count the run of backslashes immediately before it.
			bs := 0
			for j := i - 1; j >= 0 && line[j] == '\\'; j-- {
				bs++
			}
			if bs%2 == 0 {
				cut = i
				break
			}
		}
		if cut < 0 {
			b.WriteString(line)
			continue
		}
		// Drop from the % to the end of the line, but keep the line break:
		// TeX also swallows the break, yet keeping it here stops two tokens on
		// separate source lines from merging into one before NormaliseSpace
		// runs. The comment text is gone either way.
		b.WriteString(strings.TrimRight(line[:cut], " \t"))
		if strings.HasSuffix(line, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
