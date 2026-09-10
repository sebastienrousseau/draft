// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func noWarn(string, ...any) {}

func TestParseFileConfig(t *testing.T) {
	data := []byte("# comment\n; also a comment\n\nengine = \"claude\"\nReader = docling\n" +
		"extract_engine = 'ollama'\n[section]\nmodel = \"opus\"\nno equals here\nblank =\nout = \"~/drafts\"\n")
	warns := 0
	fc := parseFileConfig(data, func(string, ...any) { warns++ })
	want := map[string]string{"engine": "claude", "reader": "docling", "extract-engine": "ollama", "model": "opus", "out": "~/drafts"}
	for k, v := range want {
		if fc[k] != v {
			t.Errorf("fc[%q] = %q, want %q", k, fc[k], v)
		}
	}
	if _, ok := fc["blank"]; ok {
		t.Error("a blank value should be skipped, not stored")
	}
	if warns != 1 {
		t.Errorf("warnings = %d, want 1 (the malformed line)", warns)
	}
}

func TestFileConfigGet(t *testing.T) {
	fc := fileConfig{"engine": "claude", "blank": "   "}
	if got := fc.get("engine", "x"); got != "claude" {
		t.Errorf("get(set) = %q", got)
	}
	if got := fc.get("missing", "fallback"); got != "fallback" {
		t.Errorf("get(missing) = %q", got)
	}
	if got := fc.get("blank", "fallback"); got != "fallback" {
		t.Errorf("get(blank) = %q, want fallback", got)
	}
	var nilfc fileConfig
	if got := nilfc.get("engine", "fallback"); got != "fallback" {
		t.Errorf("nil get = %q, want fallback", got)
	}
}

func TestFileConfigDir(t *testing.T) {
	fc := fileConfig{"out": "~/drafts"}
	if got := fc.dir(noWarn, "out", "/home/u", "/def"); got != filepath.Join("/home/u", "drafts") {
		t.Errorf("dir(set) = %q", got)
	}
	if got := fc.dir(noWarn, "missing", "/home/u", "/def"); got != "/def" {
		t.Errorf("dir(unset) = %q, want /def", got)
	}
}

func TestUserConfigPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got := userConfigPath(); got != filepath.Join("/xdg", "draft", "config.toml") {
		t.Errorf("xdg path = %q", got)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	orig := userHomeDir
	t.Cleanup(func() { userHomeDir = orig })
	userHomeDir = func() (string, error) { return "/home/u", nil }
	if got := userConfigPath(); got != filepath.Join("/home/u", ".config", "draft", "config.toml") {
		t.Errorf("home path = %q", got)
	}
	userHomeDir = func() (string, error) { return "", os.ErrNotExist }
	if got := userConfigPath(); got != "" {
		t.Errorf("no home = %q, want empty", got)
	}
}

func hermeticFiles(t *testing.T) {
	t.Helper()
	t.Setenv("DRAFT_CONFIG", "")
	t.Setenv("DRAFT_NO_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())
}

func TestLoadFileConfig_Explicit(t *testing.T) {
	hermeticFiles(t)
	f := filepath.Join(t.TempDir(), "c.toml")
	if err := os.WriteFile(f, []byte("engine = \"codex\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DRAFT_CONFIG", f)
	if got := loadFileConfig(noWarn).get("engine", ""); got != "codex" {
		t.Errorf("explicit engine = %q", got)
	}
}

func TestLoadFileConfig_ExplicitMissingWarns(t *testing.T) {
	hermeticFiles(t)
	t.Setenv("DRAFT_CONFIG", filepath.Join(t.TempDir(), "nope.toml"))
	warns := 0
	if fc := loadFileConfig(func(string, ...any) { warns++ }); fc != nil {
		t.Error("missing explicit config should yield nil")
	}
	if warns != 1 {
		t.Errorf("warnings = %d, want 1", warns)
	}
}

func TestLoadFileConfig_ExplicitEmptyIsNil(t *testing.T) {
	hermeticFiles(t)
	f := filepath.Join(t.TempDir(), "e.toml")
	os.WriteFile(f, []byte("# only comments\n"), 0o644)
	t.Setenv("DRAFT_CONFIG", f)
	if loadFileConfig(noWarn) != nil {
		t.Error("an empty explicit config should yield nil")
	}
}

func TestLoadFileConfig_NoConfigDisables(t *testing.T) {
	hermeticFiles(t)
	f := filepath.Join(t.TempDir(), "c.toml")
	os.WriteFile(f, []byte("engine = \"x\"\n"), 0o644)
	t.Setenv("DRAFT_CONFIG", f)
	t.Setenv("DRAFT_NO_CONFIG", "1")
	if loadFileConfig(noWarn) != nil {
		t.Error("DRAFT_NO_CONFIG should disable the file layer")
	}
}

func TestLoadFileConfig_ProjectOverridesUser(t *testing.T) {
	hermeticFiles(t)
	xdg := t.TempDir()
	os.MkdirAll(filepath.Join(xdg, "draft"), 0o755)
	os.WriteFile(filepath.Join(xdg, "draft", "config.toml"), []byte("engine = \"user\"\nreader = \"user-reader\"\n"), 0o644)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	proj := t.TempDir()
	os.WriteFile(filepath.Join(proj, "draft.toml"), []byte("engine = \"project\"\n"), 0o644)
	t.Chdir(proj)
	fc := loadFileConfig(noWarn)
	if got := fc.get("engine", ""); got != "project" {
		t.Errorf("engine = %q, want project (project overrides user)", got)
	}
	if got := fc.get("reader", ""); got != "user-reader" {
		t.Errorf("reader = %q, want user-reader (user key survives)", got)
	}
}

func TestLoadFileConfig_AbsentIsNil(t *testing.T) {
	hermeticFiles(t)
	if loadFileConfig(noWarn) != nil {
		t.Error("no config files present should yield nil")
	}
}

func TestLoad_FilePrecedence(t *testing.T) {
	hermeticFiles(t)
	for _, k := range []string{"DRAFT_ENGINE", "DRAFT_READER", "DRAFT_MODEL_SESSION", "DRAFT_CLAUDE_MODEL", "DRAFT_MODEL", "DRAFT_DRAFTS_DIR", "DRAFT_SOURCES_DIR"} {
		t.Setenv(k, "")
	}
	orig := userHomeDir
	t.Cleanup(func() { userHomeDir = orig })
	home := t.TempDir()
	userHomeDir = func() (string, error) { return home, nil }

	proj := t.TempDir()
	os.WriteFile(filepath.Join(proj, "draft.toml"),
		[]byte("engine = \"file-engine\"\nreader = \"file-reader\"\nmodel = \"file-model\"\nout = \""+proj+"/drafts\"\n"), 0o644)
	t.Chdir(proj)

	// File value is used when env and flag are unset.
	c := Load(Flags{})
	if c.Engine != "file-engine" {
		t.Errorf("engine = %q, want file-engine", c.Engine)
	}
	if c.Reader != "file-reader" {
		t.Errorf("reader = %q, want file-reader", c.Reader)
	}
	if c.Model != "file-model" {
		t.Errorf("model = %q, want file-model", c.Model)
	}
	if c.DraftsDir != filepath.Join(proj, "drafts") {
		t.Errorf("drafts dir = %q, want %s/drafts", c.DraftsDir, proj)
	}

	// Environment overrides the file.
	t.Setenv("DRAFT_ENGINE", "env-engine")
	if c := Load(Flags{}); c.Engine != "env-engine" {
		t.Errorf("engine = %q, want env-engine (env beats file)", c.Engine)
	}

	// A flag overrides both.
	if c := Load(Flags{Reader: "flag-reader"}); c.Reader != "flag-reader" {
		t.Errorf("reader = %q, want flag-reader (flag beats file)", c.Reader)
	}
}
