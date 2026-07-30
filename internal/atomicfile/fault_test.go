// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var errFault = errors.New("simulated I/O fault")

// faultyFile wraps a real temp file and fails at a chosen step. The point of
// this package is surviving exactly these failures, and they cannot be
// provoked portably for real.
type faultyFile struct {
	*os.File
	failWrite bool
	failSync  bool
	failClose bool
}

func (f *faultyFile) Write(p []byte) (int, error) {
	if f.failWrite {
		return 0, errFault
	}
	return f.File.Write(p)
}

func (f *faultyFile) Sync() error {
	if f.failSync {
		return errFault
	}
	return f.File.Sync()
}

func (f *faultyFile) Close() error {
	err := f.File.Close()
	if f.failClose {
		return errFault
	}
	return err
}

// withFault installs a createTemp that returns files failing at the given step.
func withFault(t *testing.T, step string) {
	t.Helper()
	original := createTemp
	t.Cleanup(func() { createTemp = original })

	createTemp = func(dir, pattern string) (tempFile, error) {
		f, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		return &faultyFile{
			File:      f,
			failWrite: step == "write",
			failSync:  step == "sync",
			failClose: step == "close",
		}, nil
	}
}

func TestWriteReportsEachStagingFailure(t *testing.T) {
	for _, tc := range []struct{ step, want string }{
		{"write", "writing"},
		{"sync", "flushing"},
		{"close", "closing"},
	} {
		t.Run(tc.step, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "draft.md")
			if err := os.WriteFile(path, []byte("ORIGINAL"), 0o644); err != nil {
				t.Fatal(err)
			}
			withFault(t, tc.step)

			err := Write(path, []byte("replacement"), 0o644)
			if err == nil {
				t.Fatalf("expected a failure at the %s step", tc.step)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to name the %s step", err, tc.want)
			}
			if !errors.Is(err, errFault) {
				t.Errorf("the underlying fault was lost: %v", err)
			}

			// The whole point: the original must survive intact.
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(got) != "ORIGINAL" {
				t.Errorf("the original was damaged: %q", got)
			}
			assertNoTempFiles(t, dir)
		})
	}
}

func TestWriteSetReportsEachStagingFailure(t *testing.T) {
	for _, tc := range []struct{ step, want string }{
		{"write", "writing"},
		{"sync", "flushing"},
		{"close", "closing"},
	} {
		t.Run(tc.step, func(t *testing.T) {
			dir := t.TempDir()
			body := filepath.Join(dir, "body.md")
			if err := os.WriteFile(body, []byte("ORIGINAL"), 0o644); err != nil {
				t.Fatal(err)
			}
			withFault(t, tc.step)

			err := WriteSet(map[string][]byte{
				body:                            []byte("new body"),
				filepath.Join(dir, "final.md"):  []byte("new final"),
				filepath.Join(dir, "meta.yaml"): []byte("new meta"),
			}, 0o644)
			if err == nil {
				t.Fatalf("expected a failure at the %s step", tc.step)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to name the %s step", err, tc.want)
			}

			// Nothing may be published when the set could not be staged.
			got, readErr := os.ReadFile(body)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(got) != "ORIGINAL" {
				t.Errorf("a file was published despite the set failing: %q", got)
			}
			for _, name := range []string{"final.md", "meta.yaml"} {
				if _, statErr := os.Stat(filepath.Join(dir, name)); statErr == nil {
					t.Errorf("%s was published despite the set failing", name)
				}
			}
			assertNoTempFiles(t, dir)
		})
	}
}

// A staging failure must not leave the temp file behind either.
func TestFaultyStagingCleansUp(t *testing.T) {
	dir := t.TempDir()
	withFault(t, "write")
	_ = Write(filepath.Join(dir, "draft.md"), []byte("x"), 0o644)
	assertNoTempFiles(t, dir)
}
