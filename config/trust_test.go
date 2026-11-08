// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// trustHere trusts the draft.toml in the working directory as it stands.
func trustHere(t *testing.T) {
	t.Helper()
	if _, err := TrustProjectConfig(); err != nil {
		t.Fatal(err)
	}
}

// inProject makes a hermetic working directory holding a draft.toml with body.
func inProject(t *testing.T, body string) string {
	t.Helper()
	hermeticFiles(t)
	proj := t.TempDir()
	path := filepath.Join(proj, "draft.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)
	return path
}

func collectWarnings() (func(string, ...any), *[]string) {
	var got []string
	return func(f string, a ...any) { got = append(got, fmt.Sprintf(f, a...)) }, &got
}

// The finding (S-M3): a draft.toml in whatever folder the user runs draft
// from could pick the engine, the output folder, the style and the signing
// key. Untrusted, it must apply none of them, and say why.
func TestUntrustedProjectConfigIsIgnoredAndReported(t *testing.T) {
	path := inProject(t, "engine = \"evil\"\nout = \"/tmp/attacker\"\nstyle = \"/tmp/s.json\"\nc2pa-key = \"/tmp/k\"\n[site]\nname = \"Mallory\"\n")
	warn, got := collectWarnings()
	if fc := loadFileConfig(warn); fc != nil {
		t.Fatalf("an untrusted project config was applied: %v", fc)
	}
	if len(*got) != 1 || !strings.Contains((*got)[0], path) || !strings.Contains((*got)[0], "--trust-config") {
		t.Errorf("warnings = %q, want one naming %s and --trust-config", *got, path)
	}

	c := Load(Flags{})
	if c.Engine == "evil" || c.SignKey != "" || c.StylePath != "" || c.Site.Name != "" {
		t.Errorf("Load applied the untrusted file: engine %q key %q style %q site %q", c.Engine, c.SignKey, c.StylePath, c.Site.Name)
	}
	if !hasWarning(c, "--trust-config") {
		t.Errorf("Load warnings %q do not report the ignored file", c.Warnings)
	}
}

func TestTrustedProjectConfigApplies(t *testing.T) {
	inProject(t, "engine = \"codex\"\n")
	trustHere(t)
	warn, got := collectWarnings()
	if e := loadFileConfig(warn).get("engine", ""); e != "codex" || len(*got) != 0 {
		t.Errorf("engine = %q, warnings %q; want codex and none", e, *got)
	}
}

// Trust is in the content, not the name: an edit after trusting, by anyone,
// lapses it, and trusting again restores it.
func TestEditingATrustedConfigLapsesTheTrust(t *testing.T) {
	path := inProject(t, "engine = \"codex\"\n")
	trustHere(t)
	if err := os.WriteFile(path, []byte("engine = \"evil\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	warn, got := collectWarnings()
	if fc := loadFileConfig(warn); fc != nil {
		t.Fatalf("an edited config was applied: %v", fc)
	}
	if len(*got) != 1 || !strings.Contains((*got)[0], "changed since you trusted it") {
		t.Errorf("warnings = %q, want one saying it changed", *got)
	}

	trustHere(t)
	if e := loadFileConfig(noWarn).get("engine", ""); e != "evil" {
		t.Errorf("engine = %q after re-trusting, want evil", e)
	}
	if n := len(readTrust()); n != 1 {
		t.Errorf("trust record holds %d entries, want 1: re-trusting replaces", n)
	}
}

// Trusting one folder's draft.toml says nothing about another's with the
// same contents: the record is keyed by path.
func TestTrustIsPerPath(t *testing.T) {
	inProject(t, "engine = \"codex\"\n")
	trustHere(t)
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "draft.toml"), []byte("engine = \"codex\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(other)
	if fc := loadFileConfig(noWarn); fc != nil {
		t.Errorf("a config in another folder was trusted: %v", fc)
	}
}

// The user's own config and one they name explicitly are theirs; neither
// needs trusting.
func TestUserAndExplicitConfigsNeedNoTrust(t *testing.T) {
	hermeticFiles(t)
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if err := os.MkdirAll(filepath.Join(xdg, "draft"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "draft", "config.toml"), []byte("engine = \"user\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if e := loadFileConfig(noWarn).get("engine", ""); e != "user" {
		t.Errorf("user config engine = %q, want user", e)
	}
	explicit := filepath.Join(t.TempDir(), "draft.toml")
	if err := os.WriteFile(explicit, []byte("engine = \"named\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DRAFT_CONFIG", explicit)
	if e := loadFileConfig(noWarn).get("engine", ""); e != "named" {
		t.Errorf("explicit config engine = %q, want named", e)
	}
}

func TestTrustRecordIsPrivate(t *testing.T) {
	inProject(t, "engine = \"codex\"\n")
	trustHere(t)
	fi, err := os.Stat(trustFilePath())
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 && os.PathSeparator == '/' {
		t.Errorf("trust record mode = %v, want no group or other access", perm)
	}
}

func TestReadTrustSkipsMalformedLines(t *testing.T) {
	path := inProject(t, "engine = \"codex\"\n")
	if err := os.MkdirAll(filepath.Dir(trustFilePath()), 0o700); err != nil {
		t.Fatal(err)
	}
	junk := "not-a-digest " + path + "\n\n" + strings.Repeat("a", 64) + "\n"
	if err := os.WriteFile(trustFilePath(), []byte(junk), 0o600); err != nil {
		t.Fatal(err)
	}
	if n := len(readTrust()); n != 0 {
		t.Errorf("readTrust kept %d malformed entries", n)
	}
}

func TestTrustProjectConfigErrors(t *testing.T) {
	hermeticFiles(t)
	if _, err := TrustProjectConfig(); err == nil {
		t.Error("trusting a missing draft.toml should fail")
	}

	origWD := getwd
	t.Cleanup(func() { getwd = origWD })
	getwd = func() (string, error) { return "", errors.New("no cwd") }
	if _, err := TrustProjectConfig(); err == nil || !strings.Contains(err.Error(), "working directory") {
		t.Errorf("err = %v, want a working-directory error", err)
	}
	getwd = origWD

	origHome := userHomeDir
	t.Cleanup(func() { userHomeDir = origHome })
	userHomeDir = func() (string, error) { return "", errors.New("no home") }
	t.Setenv("XDG_CONFIG_HOME", "")
	if _, err := TrustProjectConfig(); err == nil || !strings.Contains(err.Error(), "home") {
		t.Errorf("err = %v, want a no-home error", err)
	}
	if len(readTrust()) != 0 {
		t.Error("with no home, nothing is trusted")
	}
}

func TestTrustProjectConfigReportsAnUnwritableRecord(t *testing.T) {
	inProject(t, "engine = \"codex\"\n")
	// A file where the record's directory should be.
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocker)
	if _, err := TrustProjectConfig(); err == nil {
		t.Error("an unwritable trust record should be an error")
	}
}
