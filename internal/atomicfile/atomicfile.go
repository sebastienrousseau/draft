// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package atomicfile writes a file so that a reader never observes it
// half-written.
//
// draft rewrites the user's own articles in place — --review replaces a draft,
// --frontmatter regenerates a three-file set — and a plain os.WriteFile
// truncates the destination before it writes. An interrupted write therefore
// destroys the original with nothing to fall back on, and a failure partway
// through a set leaves the three files disagreeing with each other.
package atomicfile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// tempFile is the part of *os.File this package uses. It exists so tests can
// substitute a file that fails partway through — the failures this package is
// built to survive are exactly the ones that are hardest to provoke for real.
type tempFile interface {
	io.Writer
	Sync() error
	Close() error
	Name() string
}

// createTemp is a variable for the same reason.
var createTemp = func(dir, pattern string) (tempFile, error) {
	return os.CreateTemp(dir, pattern)
}

var chmod = os.Chmod

// Write writes data to path via a temporary file in the same directory
// followed by a rename, so the destination is either the old content or the
// new one and never something in between.
//
// The temporary file is created alongside the destination because rename is
// only atomic within a single filesystem; a temp file in os.TempDir could land
// on a different one and degrade to a copy.
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := createTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("staging a write to %s: %w", path, err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup; a no-op once the rename has succeeded.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	// Flush before the rename, so a crash cannot leave a renamed but empty
	// file where the user's article used to be.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("flushing %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", path, err)
	}
	// CreateTemp always uses 0600; apply the caller's mode before publishing.
	if err := chmod(tmpName, perm); err != nil {
		return fmt.Errorf("setting mode on %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("publishing %s: %w", path, err)
	}
	return nil
}

// WriteSet writes several files as a unit: every file is staged and flushed
// before any of them is published, so a failure while staging leaves all of
// the destinations untouched.
//
// The renames themselves are not a single atomic operation — no portable
// filesystem offers that — but by the time the first one runs, the only
// remaining work is renames within a directory. That turns "one file was
// rewritten and the next failed" from an ordinary outcome into a genuinely
// exceptional one, which is what keeps a generated article set in step.
func WriteSet(files map[string][]byte, perm os.FileMode) error {
	type staged struct{ tmp, dst string }
	var pending []staged

	cleanup := func() {
		for _, s := range pending {
			_ = os.Remove(s.tmp)
		}
	}

	// Stage every file first.
	for dst, data := range files {
		tmp, err := createTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".tmp-*")
		if err != nil {
			cleanup()
			return fmt.Errorf("staging a write to %s: %w", dst, err)
		}
		pending = append(pending, staged{tmp: tmp.Name(), dst: dst})

		if _, err := tmp.Write(data); err != nil {
			_ = tmp.Close()
			cleanup()
			return fmt.Errorf("writing %s: %w", dst, err)
		}
		if err := tmp.Sync(); err != nil {
			_ = tmp.Close()
			cleanup()
			return fmt.Errorf("flushing %s: %w", dst, err)
		}
		if err := tmp.Close(); err != nil {
			cleanup()
			return fmt.Errorf("closing %s: %w", dst, err)
		}
		if err := chmod(tmp.Name(), perm); err != nil {
			cleanup()
			return fmt.Errorf("setting mode on %s: %w", dst, err)
		}
	}

	// Then publish them.
	for _, s := range pending {
		if err := os.Rename(s.tmp, s.dst); err != nil {
			cleanup()
			return fmt.Errorf("publishing %s: %w", s.dst, err)
		}
	}
	return nil
}
