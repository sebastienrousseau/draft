// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pointSourceAt rewrites the set's manifest so its one source is recorded
// as recorded, which is how a manifest from elsewhere, or a crafted one,
// arrives.
func pointSourceAt(t *testing.T, day, stem, was, recorded string) {
	t.Helper()
	path := filepath.Join(day, "provenance", stem+"-c2pa.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	from, _ := json.Marshal(was)
	to, _ := json.Marshal(recorded)
	if !strings.Contains(string(b), string(from)) {
		t.Fatalf("the manifest does not record %s", was)
	}
	b = []byte(strings.ReplaceAll(string(b), string(from), string(to)))
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A manifest now records a source relative to itself, so it no longer
// carries the user's home directory. --verify must resolve it from the
// manifest's folder, not from wherever the verifier happens to be run.
func TestVerifyResolvesASourceRelativeToTheManifest(t *testing.T) {
	day, stem, sourcePath := writeRichSet(t, "the source text", false, false)
	manifestDir := filepath.Join(day, "provenance")
	rel, err := filepath.Rel(manifestDir, sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	pointSourceAt(t, day, stem, sourcePath, filepath.ToSlash(rel))

	report, _, _, err := loadSet(filepath.Join(day, "final", stem+"-final.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sources) != 1 || !report.Sources[0].Found || !report.Sources[0].Matches {
		t.Errorf("a relative source was not found beside the manifest: %+v", report.Sources)
	}
}

// --verify opened whatever path a manifest named. A directory, a device or
// a FIFO is not a source anyone drafted from; refuse it by name rather than
// reading it, or hanging on it.
func TestVerifyRefusesANonRegularSource(t *testing.T) {
	day, stem, sourcePath := writeRichSet(t, "the source text", false, false)
	pointSourceAt(t, day, stem, sourcePath, t.TempDir())

	_, _, _, err := loadSet(filepath.Join(day, "final", stem+"-final.md"))
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("a directory named as a source was not refused: %v", err)
	}
}

// Hashing streams, so size is a bound on time rather than memory, but a
// manifest naming a huge file should be told apart from a slow verify.
func TestVerifyRefusesAnOversizedSource(t *testing.T) {
	day, stem, _ := writeRichSet(t, "the source text", false, false)
	defer func(was int64) { maxVerifySourceBytes = was }(maxVerifySourceBytes)
	maxVerifySourceBytes = 4

	_, _, _, err := loadSet(filepath.Join(day, "final", stem+"-final.md"))
	if err == nil || !strings.Contains(err.Error(), "the limit is 4") {
		t.Errorf("an oversized source was not refused: %v", err)
	}
}

// A source that is simply not here is still a note, not a failure:
// provenance travels without its inputs.
func TestDigestSourceTreatsAMissingFileAsAbsent(t *testing.T) {
	sum, found, err := digestSource(filepath.Join(t.TempDir(), "gone.pdf"))
	if sum != "" || found || err != nil {
		t.Errorf("digestSource(missing) = %q, %v, %v", sum, found, err)
	}
}
