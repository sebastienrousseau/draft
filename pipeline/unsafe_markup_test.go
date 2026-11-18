// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/engine"
)

// A quote is verbatim by design, so a source can put a script tag in the
// writer's mouth. The draft goes back to the writer like any other violation,
// and what is saved is the draft without it.
func TestUnsafeMarkupInADraftIsSentBack(t *testing.T) {
	cfg := testConfig(t)
	cfg.WriteRetries = 1
	eng := &fakeEngine{name: "fake", writer: func(call int) (string, bool) {
		if call == 1 {
			return validArticle(` The paper quotes <script>alert(1)</script> verbatim.`), false
		}
		return validArticle("."), false
	}}
	done, errText, logs := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("the retry should have recovered: %s", errText)
	}
	if !hasLog(logs, "write retry 1: contains unsafe markup: <script>") {
		t.Errorf("the retry should name the unsafe markup, logs=%v", logs)
	}
	if got := savedMarkdownWith(t, cfg.DraftsDir, "<script"); len(got) > 0 {
		t.Errorf("a saved file carries the script tag: %v", got)
	}
	if done.OutputPath == "" {
		t.Error("the clean retry was not saved")
	}
}

// When every attempt keeps the markup, the run fails, and the rescued
// needs-review Markdown is not written: it sits beside the drafts and is just as
// renderable. The raw output keeps the evidence as plain text.
func TestUnsafeMarkupIsNeverSavedAsMarkdown(t *testing.T) {
	cfg := testConfig(t)
	eng := &fakeEngine{name: "fake", writer: func(int) (string, bool) {
		return validArticle(` See [the appendix](javascript:alert(1)).`), false
	}}
	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if !strings.Contains(errText, "contains unsafe markup: javascript: URL") {
		t.Fatalf("the run should fail on the unsafe link, got %q", errText)
	}
	if !strings.Contains(errText, "Needs-review Markdown not saved") {
		t.Errorf("the skipped rescue should be reported, got %q", errText)
	}
	if got := savedMarkdownWith(t, cfg.DraftsDir, "javascript:"); len(got) > 0 {
		t.Errorf("a Markdown file carries the javascript: link: %v", got)
	}
}

// save is the last gate before disk. It checks the frontmatter as well as the
// body, because the frontmatter is generated from the body and the site
// configuration, and a template renders its values as raw HTML.
func TestSaveRefusesUnsafeMarkup(t *testing.T) {
	for _, tc := range []struct {
		name, body, site string
	}{
		{"body", validArticle(` See [the appendix](javascript:alert(1)).`), ""},
		{"frontmatter", validArticle("."), `</title><script>alert(1)</script>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig(t)
			cfg.Site.Name = tc.site
			outputDir := t.TempDir()
			_, _, err := NewRunner(cfg, nil, nil).save(context.Background(), outputDir, tc.body, nil)
			if err == nil || !strings.Contains(err.Error(), "unsafe markup") {
				t.Fatalf("save should refuse unsafe markup, got %v", err)
			}
			if got := savedMarkdownWith(t, outputDir, ""); len(got) > 0 {
				t.Errorf("a refused save left files behind: %v", got)
			}
		})
	}
}

// savedMarkdownWith lists the Markdown and YAML files under dir that contain
// needle ("" matches any).
func savedMarkdownWith(t *testing.T, dir, needle string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if ext := filepath.Ext(path); ext != ".md" && ext != ".yaml" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(b), needle) {
			found = append(found, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}
