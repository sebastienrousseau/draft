// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// setFiles lists every file left in the three folders of a set, temporary
// files included: a staged file that was never cleaned up is debris too.
func setFiles(t *testing.T, outputDir string) []string {
	t.Helper()
	var out []string
	for _, dir := range []string{"source", "yaml", "final"} {
		entries, err := os.ReadDir(filepath.Join(outputDir, dir))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		for _, e := range entries {
			out = append(out, dir+"/"+e.Name())
		}
	}
	return out
}

// A draft is written as a set or not at all. Whichever folder refuses the
// write, a failed save must leave none of the three files behind: a body with
// no final document is an article the user cannot tell was never finished.
func TestFailedSaveLeavesNoPartialSet(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory mode bits do not restrict writes on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	for _, readOnly := range []string{"yaml", "final"} {
		t.Run(readOnly, func(t *testing.T) {
			outputDir := t.TempDir()
			for _, d := range []string{"source", "yaml", "final"} {
				if err := os.MkdirAll(filepath.Join(outputDir, d), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			locked := filepath.Join(outputDir, readOnly)
			if err := os.Chmod(locked, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

			r := NewRunner(testConfig(t), nil, nil)
			if _, _, err := r.save(context.Background(), outputDir, validArticle("."), nil); err == nil {
				t.Fatal("expected save to fail")
			}
			if left := setFiles(t, outputDir); len(left) != 0 {
				t.Errorf("a failed save left a partial set: %v", left)
			}
		})
	}
}

// The renames that publish a staged set are not one atomic step, so a failure
// between them is rare but possible. The files already published must be
// taken back, or the set is partial in exactly the way staging prevents.
func TestFailedPublishTakesBackWhatItRenamed(t *testing.T) {
	orig := writeSet
	t.Cleanup(func() { writeSet = orig })
	writeSet = func(files map[string][]byte, perm os.FileMode) error {
		for path, data := range files {
			if err := os.WriteFile(path, data, perm); err != nil {
				return err
			}
			break // publish one file, then fail as a later rename would
		}
		return errors.New("rename failed")
	}

	outputDir := t.TempDir()
	r := NewRunner(testConfig(t), nil, nil)
	if _, _, err := r.save(context.Background(), outputDir, validArticle("."), nil); err == nil {
		t.Fatal("expected save to fail")
	}
	if left := setFiles(t, outputDir); len(left) != 0 {
		t.Errorf("a failed publish left a partial set: %v", left)
	}
}

// Rolling back must never remove a file this save did not claim: a stem that
// is bumped past a squatter leaves the squatter where it was.
func TestFailedSaveKeepsOtherSets(t *testing.T) {
	orig := writeSet
	t.Cleanup(func() { writeSet = orig })
	writeSet = func(map[string][]byte, os.FileMode) error { return errors.New("disk full") }

	outputDir := t.TempDir()
	yamlDir := filepath.Join(outputDir, "yaml")
	if err := os.MkdirAll(yamlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := validArticle(".")
	squatter := filepath.Join(yamlDir, datedStem(t, body)+"-frontmatter.yaml")
	if err := os.WriteFile(squatter, []byte("squatter"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRunner(testConfig(t), nil, nil)
	if _, _, err := r.save(context.Background(), outputDir, body, nil); err == nil {
		t.Fatal("expected save to fail")
	}
	if got, err := os.ReadFile(squatter); err != nil || string(got) != "squatter" {
		t.Errorf("rollback touched another set's file: %q, %v", got, err)
	}
	if left := setFiles(t, outputDir); len(left) != 1 {
		t.Errorf("want only the squatter left, got %v", left)
	}
}
