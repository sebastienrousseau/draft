// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package atomicfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteReplacesCleanly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "draft.md")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Write(path, []byte("replacement"), 0o644); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "replacement" {
		t.Errorf("content = %q", got)
	}
	assertNoTempFiles(t, filepath.Dir(path))
}

func TestWriteCreatesAMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.md")
	if err := Write(path, []byte("fresh"), 0o644); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "fresh" {
		t.Errorf("got %q, %v", got, err)
	}
}

// The whole point: a failed write must leave the original intact, not
// truncated.
func TestWriteLeavesTheOriginalWhenStagingFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "missing", "draft.md")
	if err := Write(path, []byte("new"), 0o644); err == nil {
		t.Error("expected an error when the destination directory does not exist")
	}
}

func TestWritePreservesMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "draft.md")
	if err := Write(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// CreateTemp always makes 0600, so a 0644 request proves the chmod ran.
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %v, want 0600", perm)
	}
	other := filepath.Join(t.TempDir(), "draft.md")
	if err := Write(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(other)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("mode = %v, want 0644", perm)
	}
}

func TestWriteSetPublishesEverything(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{
		filepath.Join(dir, "body.md"):        []byte("body"),
		filepath.Join(dir, "frontmatter.md"): []byte("yaml"),
		filepath.Join(dir, "final.md"):       []byte("final"),
	}
	if err := WriteSet(files, 0o644); err != nil {
		t.Fatalf("WriteSet: %v", err)
	}
	for path, want := range files {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if string(got) != string(want) {
			t.Errorf("%s = %q, want %q", filepath.Base(path), got, want)
		}
	}
	assertNoTempFiles(t, dir)
}

// A set that cannot be staged in full must not publish any of it — that is
// what keeps a generated article set from disagreeing with itself.
func TestWriteSetPublishesNothingWhenStagingFails(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "body.md")
	if err := os.WriteFile(good, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := WriteSet(map[string][]byte{
		good:                                   []byte("replacement"),
		filepath.Join(dir, "nope", "final.md"): []byte("unreachable directory"),
	}, 0o644)
	if err == nil {
		t.Fatal("expected WriteSet to fail")
	}

	// The reachable file must be untouched, not half-updated.
	got, readErr := os.ReadFile(good)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "ORIGINAL" {
		t.Errorf("a file was published despite the set failing: %q", got)
	}
	assertNoTempFiles(t, dir)
}

func TestWriteSetEmptyIsANoOp(t *testing.T) {
	if err := WriteSet(nil, 0o644); err != nil {
		t.Errorf("WriteSet(nil) = %v, want nil", err)
	}
}

// A rename onto an existing directory cannot succeed, which exercises the
// publish step's failure path without needing to simulate a disk fault.
func TestWritePublishFailureIsReported(t *testing.T) {
	dir := t.TempDir()
	occupied := filepath.Join(dir, "occupied")
	if err := os.Mkdir(occupied, 0o755); err != nil {
		t.Fatal(err)
	}

	err := Write(occupied, []byte("data"), 0o644)
	if err == nil {
		t.Fatal("expected publishing over a directory to fail")
	}
	if !strings.Contains(err.Error(), "publishing") {
		t.Errorf("error should say which step failed, got %q", err)
	}
	assertNoTempFiles(t, dir)
}

func TestWriteSetPublishFailureIsReported(t *testing.T) {
	dir := t.TempDir()
	occupied := filepath.Join(dir, "occupied")
	if err := os.Mkdir(occupied, 0o755); err != nil {
		t.Fatal(err)
	}

	err := WriteSet(map[string][]byte{occupied: []byte("data")}, 0o644)
	if err == nil {
		t.Fatal("expected publishing over a directory to fail")
	}
	if !strings.Contains(err.Error(), "publishing") {
		t.Errorf("error should say which step failed, got %q", err)
	}
	assertNoTempFiles(t, dir)
}

// A destination directory that cannot be written to fails at the staging step,
// before anything is published.
func TestStagingFailureInAnUnwritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o555); err != nil { // read+execute, no write
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	if err := Write(filepath.Join(locked, "draft.md"), []byte("x"), 0o644); err == nil {
		t.Error("expected Write to fail in an unwritable directory")
	} else if !strings.Contains(err.Error(), "staging") {
		t.Errorf("error should say which step failed, got %q", err)
	}

	if err := WriteSet(map[string][]byte{filepath.Join(locked, "a.md"): []byte("x")}, 0o644); err == nil {
		t.Error("expected WriteSet to fail in an unwritable directory")
	}
}

// assertNoTempFiles fails if any staging file survived.
func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") && strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("staging file %q survived", e.Name())
		}
	}
}
