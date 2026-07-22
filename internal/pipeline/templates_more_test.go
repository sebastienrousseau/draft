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

func TestLoadTemplatesStripsHeadings(t *testing.T) {
	home := t.TempDir()
	tdir := filepath.Join(home, "Drop", "Drafts", "Templates") // the fallback dir
	if err := os.MkdirAll(tdir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Templates carry headings; these must NOT survive into the calibration block,
	// or a literal model copies them into unrelated drafts.
	body := "# A Very Specific Template Heading\n\nStyle sample body goes here.\n\n## Another Template Heading\n\nMore prose."
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		if err := os.WriteFile(filepath.Join(tdir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := loadTemplates(config.Config{HomeDir: home, DraftsDir: filepath.Join(home, "Drop", "Drafts")})
	if got == "" || !strings.Contains(got, "Style sample body goes here.") {
		t.Errorf("expected a prose style sample, got %q", got[:min(80, len(got))])
	}
	if strings.Contains(got, "A Very Specific Template Heading") || strings.Contains(got, "Another Template Heading") {
		t.Errorf("template headings must be stripped from the style sample: %q", got)
	}
	if strings.Contains(got, "Heading outline") {
		t.Errorf("the heading outline section should no longer be emitted")
	}
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
