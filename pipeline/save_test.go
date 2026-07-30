// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The output trio is uniquified as a set: a leftover in any one folder must
// bump all three names together, so the files can never desync.
func TestSaveUniquifiesTheWholeSet(t *testing.T) {
	cfg := testConfig(t)
	outputDir := t.TempDir()
	r := NewRunner(cfg, nil, nil)

	// Only the yaml sibling pre-exists. The body and final names are free, but
	// the set must still move on rather than reuse the stem.
	yamlDir := filepath.Join(outputDir, "yaml")
	if err := os.MkdirAll(yamlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := validArticle(".")
	stem := datedStem(t, body)
	if err := os.WriteFile(filepath.Join(yamlDir, stem+"-frontmatter.yaml"), []byte("squatter"), 0o644); err != nil {
		t.Fatal(err)
	}

	path, words, err := r.save(outputDir, body)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if words == 0 {
		t.Error("word count was not reported")
	}
	if !strings.Contains(filepath.Base(path), "-2-final.md") {
		t.Errorf("expected the set to be bumped to -2, got %s", filepath.Base(path))
	}
	// The squatter must be untouched.
	got, err := os.ReadFile(filepath.Join(yamlDir, stem+"-frontmatter.yaml"))
	if err != nil || string(got) != "squatter" {
		t.Errorf("pre-existing sibling was overwritten: %q, %v", got, err)
	}
}

// Two saves of the same article must not collide: the body file is claimed
// with O_EXCL, so the second run takes the next stem rather than racing on a
// check-then-write.
func TestSaveClaimsTheBodyExclusively(t *testing.T) {
	cfg := testConfig(t)
	outputDir := t.TempDir()
	r := NewRunner(cfg, nil, nil)
	body := validArticle(".")

	first, _, err := r.save(outputDir, body)
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	second, _, err := r.save(outputDir, body)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if first == second {
		t.Fatalf("both saves produced %s", first)
	}

	// Every file of both sets must exist and be distinct.
	seen := map[string]bool{}
	for _, dir := range []string{"source", "yaml", "final"} {
		entries, err := os.ReadDir(filepath.Join(outputDir, dir))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			t.Errorf("%s/ holds %d files, want 2", dir, len(entries))
		}
		for _, e := range entries {
			if seen[dir+"/"+e.Name()] {
				t.Errorf("duplicate file %s/%s", dir, e.Name())
			}
			seen[dir+"/"+e.Name()] = true
		}
	}
}

// save must fail rather than spin when it cannot create the body file at all.
func TestSaveFailsWhenTheOutputTreeIsUnwritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	cfg := testConfig(t)
	outputDir := t.TempDir()
	r := NewRunner(cfg, nil, nil)

	srcDir := filepath.Join(outputDir, "source")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(srcDir, 0o500); err != nil { // read+execute, no write
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(srcDir, 0o755) })

	if _, _, err := r.save(outputDir, validArticle(".")); err == nil {
		t.Error("expected save to fail on an unwritable source directory")
	} else if errors.Is(err, os.ErrExist) {
		t.Errorf("a permission failure must not be mistaken for a name collision: %v", err)
	}
}

// saveFailure reports only the files it actually wrote: pointing the user at a
// rescued draft that was never saved sends them looking for nothing.
func TestSaveFailureReportsWhatItCouldNotWrite(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	cfg := testConfig(t)
	outputDir := t.TempDir()
	if err := os.Chmod(outputDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(outputDir, 0o755) })

	r := NewRunner(cfg, nil, nil)
	err := r.saveFailure(outputDir, validArticle("."), errors.New("rules broke"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "could not be saved") {
		t.Errorf("the failure to rescue the draft must be reported, got %q", err)
	}
	if strings.Contains(err.Error(), "\nRaw output saved:") {
		t.Errorf("reported a save that did not happen: %q", err)
	}
	// The original cause has to survive wrapping.
	if !strings.Contains(err.Error(), "rules broke") {
		t.Errorf("the underlying error was lost: %q", err)
	}
}

// datedStem mirrors the naming save derives from the article title.
func datedStem(t *testing.T, body string) string {
	t.Helper()
	return time.Now().Format("2006-01-02") + "-" + slugify(extractTitle(body))
}
