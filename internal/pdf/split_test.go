// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pdf

import (
	"strings"
	"testing"
)

func TestSplitSectionsParagraphCut(t *testing.T) {
	// A long block with paragraph breaks should be cut on a blank line.
	para := strings.Repeat("word ", 200) // ~1000 chars
	block := para + "\n\n" + para + "\n\n" + para + "\n\n" + para + "\n\n" + para
	secs := SplitSections("f.txt", block)
	if len(secs) < 2 {
		t.Fatalf("expected multiple sections from a >MaxSectionChars block, got %d", len(secs))
	}
	for _, s := range secs {
		if len(s.Body) > MaxSectionChars {
			t.Errorf("section exceeds cap: %d", len(s.Body))
		}
	}
}

func TestSplitSectionsSentenceCut(t *testing.T) {
	// No paragraph breaks, but sentence boundaries: should cut on ". ".
	sentence := strings.Repeat("a", 80) + ". "
	block := strings.Repeat(sentence, 80) // > MaxSectionChars, no blank lines
	secs := SplitSections("f.txt", block)
	if len(secs) < 2 {
		t.Fatalf("expected a sentence-boundary cut, got %d sections", len(secs))
	}
}

func TestSplitSectionsHardCut(t *testing.T) {
	// No paragraph or sentence boundaries: a hard cut at MaxSectionChars.
	block := strings.Repeat("x", MaxSectionChars*2+100)
	secs := SplitSections("f.txt", block)
	if len(secs) < 2 {
		t.Fatalf("expected hard cut, got %d", len(secs))
	}
}

func TestSplitSectionsKeepsHeadings(t *testing.T) {
	text := "Introduction\nIntro body.\nMethod\nMethod body.\nResults\nResults body."
	secs := SplitSections("f.txt", text)
	if len(secs) < 3 {
		t.Errorf("expected a section per heading, got %d", len(secs))
	}
}
