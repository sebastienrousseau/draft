// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/internal/config"
)

func TestLoadTemplatesMultipleFilesAndHeadings(t *testing.T) {
	home := t.TempDir()
	tdir := filepath.Join(home, "Drop", "Drafts", "Templates") // the fallback dir
	if err := os.MkdirAll(tdir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Many headings to exercise the heading-count cap, across several files to
	// exercise modTime sorting and the file cap.
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString("# Heading\n")
	}
	sb.WriteString("\nStyle sample body goes here.")
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		if err := os.WriteFile(filepath.Join(tdir, name), []byte(sb.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := loadTemplates(config.Config{HomeDir: home, DraftsDir: filepath.Join(home, "Drop", "Drafts")})
	if got == "" || !strings.Contains(got, "Style sample") {
		t.Errorf("expected a template block, got %q", got[:min(60, len(got))])
	}
	// Only the file cap worth of examples should be present.
	if strings.Count(got, "Template example:") > maxTemplateFiles {
		t.Errorf("too many template examples included")
	}
}

func TestEmitNilChannel(t *testing.T) {
	// A Runner with a nil event channel must not panic when emitting.
	r := NewRunner(config.Config{}, nil, nil)
	r.emit(LogEvent("ignored"))
	r.log("ignored")
	r.phase(0, "running")
}

func TestLoadTemplatesLargeContentClips(t *testing.T) {
	home := t.TempDir()
	tdir := filepath.Join(home, "Drop", "Templates")
	os.MkdirAll(tdir, 0o755)
	big := "# H\n\n" + strings.Repeat("sample body text ", 400) // > maxTemplateChars
	os.WriteFile(filepath.Join(tdir, "big.md"), []byte(big), 0o644)
	got := loadTemplates(config.Config{HomeDir: home, DraftsDir: filepath.Join(home, "Drop", "Drafts")})
	if len(got) == 0 || len(got) > maxTemplateChars+500 {
		t.Errorf("large template should be clipped near the char budget, len=%d", len(got))
	}
}
