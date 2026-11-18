// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sebastienrousseau/draft/claims"
)

// skipWithoutPOSIX skips a test that relies on POSIX modes or unprivileged
// symlinks, neither of which Windows offers.
func skipWithoutPOSIX(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes and symlinks are not modelled on Windows")
	}
}

// failedOutputName is the name a run's raw output is first offered.
func failedOutputName() string {
	return time.Now().Format("2006-01-02") + "-failed-output.txt"
}

// The raw output used to go to a fixed name, opened with a plain write. A
// symlink planted at that name redirected the write to any file the user
// could write, and its contents were the model's output.
func TestSaveFailureDoesNotFollowASymlink(t *testing.T) {
	skipWithoutPOSIX(t)
	outputDir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("precious"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(outputDir, failedOutputName())); err != nil {
		t.Fatal(err)
	}

	r := NewRunner(testConfig(t), nil, nil)
	err := r.saveFailure(outputDir, "not an article", errors.New("rules broke"))
	if got, _ := os.ReadFile(victim); string(got) != "precious" {
		t.Fatalf("the write followed the symlink: victim now holds %q", got)
	}
	if err == nil || !strings.Contains(err.Error(), "Raw output saved: ") {
		t.Fatalf("the raw output was not saved under another name: %v", err)
	}
}

// Raw model output, a needs-review draft and the claim ledger are scratch
// files holding text from the user's documents, not publications, so other
// local users have no business reading them.
func TestScratchFilesArePrivate(t *testing.T) {
	skipWithoutPOSIX(t)
	outputDir := t.TempDir()
	r := NewRunner(testConfig(t), nil, nil)
	_ = r.saveFailure(outputDir, validArticle("."), errors.New("rules broke"))
	r.saveLedger(filepath.Join(outputDir, "ledger.md"), []claims.Record{keptRecord}, 0)

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("want raw output, needs-review copy and ledger, got %d file(s)", len(entries))
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("%s has mode %v, want 0600", e.Name(), perm)
		}
	}
}

// A second failure on the same day used to overwrite the first one's raw
// output, which was the only copy of it.
func TestSaveFailureKeepsAnEarlierFailure(t *testing.T) {
	outputDir := t.TempDir()
	first := filepath.Join(outputDir, failedOutputName())
	if err := os.WriteFile(first, []byte("first failure"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(testConfig(t), nil, nil)
	_ = r.saveFailure(outputDir, "second failure", errors.New("rules broke"))
	if got, _ := os.ReadFile(first); string(got) != "first failure" {
		t.Errorf("the earlier raw output was overwritten: %q", got)
	}
}
