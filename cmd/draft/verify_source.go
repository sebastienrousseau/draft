// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// maxVerifySourceBytes bounds the source --verify will hash. Hashing
// streams, so this is a bound on time, not memory: generous enough for any
// paper, and small enough that a manifest naming a disk image fails with a
// reason instead of looking like a hang. A variable so the refusal can be
// tested without a gigabyte of fixture.
var maxVerifySourceBytes int64 = 1 << 30

// sourceResolver returns the resolver --verify hands CheckArticle for the
// manifest at manifestPath, and a function reporting the first source it
// refused to read. CheckArticle's resolver can only say found or not, and a
// refused source is neither: it is a manifest pointing somewhere it should
// not, which the user needs to hear about by name.
func sourceResolver(manifestPath string) (resolve func(string) (string, bool), refused func() error) {
	var first error
	resolve = func(recorded string) (string, bool) {
		sum, found, err := digestSource(sourceOnDisk(manifestPath, recorded))
		if err != nil && first == nil {
			first = err
		}
		return sum, found
	}
	return resolve, func() error { return first }
}

// sourceOnDisk is where a recorded source should be. Manifests written since
// 0.0.49 record it relative to the manifest's own folder, with forward
// slashes, so it resolves wherever the set and its sources were moved
// together; older ones recorded an absolute path, used as it stands.
func sourceOnDisk(manifestPath, recorded string) string {
	p := filepath.FromSlash(recorded)
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(filepath.Dir(manifestPath), p)
}

// digestSource hashes a source if it is on disk. A missing file is absent,
// not an error: provenance travels without its inputs. Anything that is not
// a regular file, or is over maxVerifySourceBytes, is refused before it is
// opened, because opening a FIFO for reading blocks until a writer appears.
func digestSource(path string) (sum string, found bool, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false, nil
	}
	if err := checkSourceInfo(path, info); err != nil {
		return "", false, err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false, nil
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	// The limit holds even if the file grows after the Stat; a digest of a
	// truncated read then reports a mismatch rather than reading on.
	if _, err := io.Copy(h, io.LimitReader(f, maxVerifySourceBytes)); err != nil {
		return "", false, nil
	}
	return hex.EncodeToString(h.Sum(nil)), true, nil
}

func checkSourceInfo(path string, info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source %s is not a regular file (%s); refusing to read it", path, info.Mode().Type())
	}
	if info.Size() > maxVerifySourceBytes {
		return fmt.Errorf("source %s is %d bytes; the limit is %d", path, info.Size(), maxVerifySourceBytes)
	}
	return nil
}
