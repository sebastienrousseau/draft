package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebastienrousseau/draft/internal/config"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Hello, World!":      "hello-world",
		"  Trim -- Dashes  ": "trim-dashes",
		"":                   "draft-article",
		"!!!":                "draft-article",
		"UPPER Case 123":     "upper-case-123",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlugifyLongTruncates(t *testing.T) {
	long := ""
	for i := 0; i < 200; i++ {
		long += "a"
	}
	if got := slugify(long); len(got) > 90 {
		t.Errorf("slug too long: %d", len(got))
	}
}

func TestExtractTitle(t *testing.T) {
	if got := extractTitle("# My Title\n\nbody"); got != "My Title" {
		t.Errorf("got %q", got)
	}
	if got := extractTitle("no title"); got != "draft-article" {
		t.Errorf("fallback title wrong: %q", got)
	}
}

func TestCleanOutputAndStripThinking(t *testing.T) {
	raw := "\x1b[31mpreamble\x1b[0m\nnoise\n# Real Title\n\nbody\x00"
	cleaned := cleanOutput(raw)
	if wantAbsent := "\x1b["; contains(cleaned, wantAbsent) {
		t.Error("ANSI not stripped")
	}
	md := stripThinking(cleaned)
	if md[:2] != "# " {
		t.Errorf("stripThinking should start at H1, got %q", md[:min(20, len(md))])
	}
}

func TestStripThinkingWithTags(t *testing.T) {
	got := stripThinking("<think>reasoning</think>\n# Title\n\nbody")
	if got[:7] != "# Title" {
		t.Errorf("think block not removed: %q", got)
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	if got := uniquePath(p); got != p {
		t.Errorf("non-existent path should be returned as-is")
	}
	os.WriteFile(p, []byte("x"), 0o644)
	got := uniquePath(p)
	if got == p || filepath.Base(got) != "a-2.md" {
		t.Errorf("expected a-2.md, got %q", got)
	}
}

func TestShortPath(t *testing.T) {
	cfg := config.Config{HomeDir: "/home/seb"}
	if got := shortPath(cfg, "/home/seb/Drop/x.md"); got != "~/Drop/x.md" {
		t.Errorf("got %q", got)
	}
	if got := shortPath(cfg, "/other/x.md"); got != "/other/x.md" {
		t.Errorf("non-home path should be unchanged, got %q", got)
	}
}

func TestLoadTemplates(t *testing.T) {
	// No template dir -> empty (built-in style used downstream).
	if got := loadTemplates(config.Config{HomeDir: t.TempDir(), DraftsDir: t.TempDir()}); got != "" {
		t.Errorf("expected empty templates, got %q", got)
	}
	// A populated Drop/Templates dir -> non-empty calibration block.
	home := t.TempDir()
	tdir := filepath.Join(home, "Drop", "Templates")
	os.MkdirAll(tdir, 0o755)
	os.WriteFile(filepath.Join(tdir, "sample.md"), []byte("# Heading\n\nStyle sample body."), 0o644)
	got := loadTemplates(config.Config{HomeDir: home, DraftsDir: filepath.Join(home, "Drop", "Drafts")})
	if got == "" || !contains(got, "Style sample") {
		t.Errorf("expected template block, got %q", got)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
