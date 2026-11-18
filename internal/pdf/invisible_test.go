// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pdf

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hidden is a sentence carrying an instruction in tag characters, a bidi
// override and zero-width characters: what a reader sees and what a model
// reads differ.
const hidden = "The model\u200B reached\U000E0049\U000E0047\U000E004E\U000E004F\U000E0052\U000E0045 a \u202Escore\u202C of\u2066 0.82\u2069.\uFEFF"

const visible = "The model reached a score of 0.82."

// Every reader's text goes through NormaliseSpace, so stripping there is what
// makes the prompt and quote verification see the same, visible, text.
func TestExtractStripsInvisibleCharacters(t *testing.T) {
	for _, ext := range []string{".txt", ".md", ".tex"} {
		t.Run(ext, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source"+ext)
			if err := os.WriteFile(path, []byte(hidden), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := Extract(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if got != visible {
				t.Errorf("Extract = %+q, want %+q", got, visible)
			}
		})
	}
}

func TestSplitSectionsStripsInvisibleCharacters(t *testing.T) {
	sections := SplitSections("paper.txt", "Abstract\n"+hidden)
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(sections))
	}
	if !strings.HasSuffix(sections[0].Body, visible) {
		t.Errorf("section body = %+q, want it to end %+q", sections[0].Body, visible)
	}
}
