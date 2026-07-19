package pdf

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractTextAndMarkdown(t *testing.T) {
	dir := t.TempDir()
	for _, ext := range []string{".txt", ".md"} {
		p := filepath.Join(dir, "f"+ext)
		if err := os.WriteFile(p, []byte("hello\r\n\r\n\r\n\r\nworld"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := Extract(context.Background(), p)
		if err != nil {
			t.Fatalf("%s: %v", ext, err)
		}
		if !strings.HasPrefix(got, "hello") || strings.Contains(got, "\r") {
			t.Errorf("%s: not normalised: %q", ext, got)
		}
	}
}

func TestExtractUnsupported(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.xyz")
	os.WriteFile(p, []byte("x"), 0o644)
	if _, err := Extract(context.Background(), p); err == nil {
		t.Error("unsupported extension should error")
	}
}

func TestExtractMissingFile(t *testing.T) {
	if _, err := Extract(context.Background(), "/no/such/file.txt"); err == nil {
		t.Error("missing file should error")
	}
}

func TestSplitSectionsSingleShortBlock(t *testing.T) {
	secs := SplitSections("f.txt", "Just one short paragraph with no headings.")
	if len(secs) != 1 {
		t.Fatalf("expected 1 section, got %d", len(secs))
	}
	if !strings.Contains(secs[0].Label, "section 1") {
		t.Errorf("bad label: %q", secs[0].Label)
	}
}

func TestSplitSectionsEmpty(t *testing.T) {
	if len(SplitSections("f.txt", "   \n\n  ")) != 0 {
		t.Error("blank input should yield no sections")
	}
	if len(SplitSections("f.txt", "References\n[1] foo")) != 0 {
		t.Error("reference-only input should yield no sections")
	}
}
