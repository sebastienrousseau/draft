// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pdf

import (
	"strings"
	"testing"
)

func TestSplitSectionsBounds(t *testing.T) {
	long := "Abstract\n" + strings.Repeat("word ", 2000) + "\nIntroduction\n" + strings.Repeat("more ", 300)
	secs := SplitSections("paper.pdf", long)
	if len(secs) < 2 {
		t.Fatalf("expected multiple sections, got %d", len(secs))
	}
	for _, s := range secs {
		if len(s.Body) > MaxSectionChars {
			t.Errorf("section %q exceeds cap: %d chars", s.Label, len(s.Body))
		}
	}
}

func TestSplitSectionsDropsReferences(t *testing.T) {
	text := "Introduction\nThe core idea is simple.\n\nReferences\n[1] Someone, 2020."
	secs := SplitSections("paper.pdf", text)
	joined := ""
	for _, s := range secs {
		joined += s.Body
	}
	if strings.Contains(joined, "Someone, 2020") {
		t.Errorf("reference section should be dropped, got: %q", joined)
	}
}

func TestNormaliseSpace(t *testing.T) {
	if got := NormaliseSpace("a\r\nb\r\n\n\n\n\nc"); !strings.Contains(got, "a\nb") {
		t.Errorf("CRLF not normalised: %q", got)
	}
}
