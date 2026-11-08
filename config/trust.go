// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A draft.toml in the working directory arrives with whatever folder the user
// happens to be in: a cloned repository, an unpacked archive, a shared drive.
// It can choose the engine, the output folder (whose Templates are put into
// the writing prompt), the house style and the C2PA signing key, so it is
// read only once the user has said they trust it as it now stands. The
// record is its SHA-256 beside its absolute path, so an edit — by anyone —
// lapses the trust until it is given again, as direnv does for .envrc.

// projectConfigName is the file looked for in the working directory.
const projectConfigName = "draft.toml"

// trustState is what the trust record says about one project config.
type trustState int

const (
	untrusted trustState = iota // never trusted
	changed                     // trusted once, but edited since
	trusted                     // trusted exactly as it stands
)

// trustFilePath is the trust record, kept beside the user config so it is
// the user's own and never part of a project. Empty when no home is known,
// which leaves every project config untrusted.
func trustFilePath() string {
	u := userConfigPath()
	if u == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(u), "trusted")
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// readTrust returns the trust record as path → digest. A missing or
// unreadable record trusts nothing; a malformed line is skipped.
func readTrust() map[string]string {
	records := map[string]string{}
	p := trustFilePath()
	if p == "" {
		return records
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return records
	}
	for _, line := range strings.Split(string(data), "\n") {
		// The path comes last so one containing spaces survives the split.
		sum, path, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok && len(sum) == sha256.Size*2 && path != "" {
			records[path] = sum
		}
	}
	return records
}

// trustOf reports whether the config at path, holding data, is trusted.
func trustOf(path string, data []byte) trustState {
	sum, ok := readTrust()[path]
	switch {
	case !ok:
		return untrusted
	case sum != digest(data):
		return changed
	default:
		return trusted
	}
}

// readProjectConfig reads the draft.toml at path only if it is trusted as it
// stands. One that is present but untrusted is reported, never applied: the
// user should know a file is being ignored, and how to stop ignoring it.
func readProjectConfig(path string, warn func(string, ...any)) fileConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	switch trustOf(path, data) {
	case trusted:
		return parseFileConfig(data, warn)
	case changed:
		warn("%s has changed since you trusted it; ignoring it until you run draft --trust-config again", path)
	default:
		warn("ignoring %s: a config found in the working directory can choose the engine, the output folder and the signing key, so draft reads it only once you run draft --trust-config", path)
	}
	return nil
}

// TrustProjectConfig records the draft.toml in the working directory as
// trusted in its current form, and returns its path. Any later edit lapses
// the trust. It replaces an earlier record for the same path rather than
// adding a second one.
func TrustProjectConfig() (string, error) {
	record := trustFilePath()
	if record == "" {
		return "", errors.New("no home directory, so there is nowhere to record the trust")
	}
	wd, err := getwd()
	if err != nil {
		return "", fmt.Errorf("working directory: %w", err)
	}
	path := filepath.Join(wd, projectConfigName)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	records := readTrust()
	records[path] = digest(data)
	return path, writeTrust(record, records)
}

// writeTrust replaces the trust record with records, sorted so the file is
// stable and easy to read. It is private to the user: it decides what draft
// will act on.
func writeTrust(record string, records map[string]string) error {
	paths := make([]string, 0, len(records))
	for p := range records {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, p := range paths {
		b.WriteString(records[p] + " " + p + "\n")
	}
	if err := os.MkdirAll(filepath.Dir(record), 0o700); err != nil {
		return err
	}
	return os.WriteFile(record, []byte(b.String()), 0o600)
}
