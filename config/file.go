// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
)

// fileConfig holds the scalar defaults read from a draft config file. Keys are
// normalised to lower-case with hyphens (engine, extract-engine, reader, model,
// out, sources-dir, style, ...). It sits between environment variables and the
// built-in defaults: a value here is used only when the matching environment
// variable and command-line flag are both unset.
type fileConfig map[string]string

// get returns the configured value for key, or fallback when it is unset or
// blank. It is nil-safe: with no config file loaded the receiver is nil and
// every lookup returns its fallback, so resolution is byte-identical to having
// no file at all.
func (fc fileConfig) get(key, fallback string) string {
	if v, ok := fc[key]; ok {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return fallback
}

// dir resolves a directory-valued key: an absolute, tilde-expanded path when
// the key is set, otherwise the caller's default. A relative path is left to
// absDir at the call site (it resolves against the process working directory).
func (fc fileConfig) dir(warn func(string, ...any), key, home, fallback string) string {
	raw := fc.get(key, "")
	if raw == "" {
		return fallback
	}
	return absDir(warn, "draft config "+key, expandHome(raw, home))
}

// loadFileConfig discovers and merges draft's config files. Precedence within
// the file layer: an explicit DRAFT_CONFIG wins outright; otherwise a project
// draft.toml in the working directory overrides a user config under
// $XDG_CONFIG_HOME/draft (or ~/.config/draft). DRAFT_NO_CONFIG disables the
// layer entirely. A missing discovered file is silent; a missing DRAFT_CONFIG
// is a warning, because the user asked for it by name.
func loadFileConfig(warn func(string, ...any)) fileConfig {
	if envBool("DRAFT_NO_CONFIG") {
		return nil
	}

	if explicit := strings.TrimSpace(os.Getenv("DRAFT_CONFIG")); explicit != "" {
		data, err := os.ReadFile(explicit)
		if err != nil {
			warn("config file %s: %v; ignoring it", explicit, err)
			return nil
		}
		fc := parseFileConfig(data, warn)
		if len(fc) == 0 {
			return nil
		}
		return fc
	}

	fc := fileConfig{}
	// User config first, project config last, so the project overrides the user.
	if u := userConfigPath(); u != "" {
		merge(fc, readFileConfig(u, warn))
	}
	if wd, err := getwd(); err == nil && wd != "" {
		merge(fc, readFileConfig(filepath.Join(wd, "draft.toml"), warn))
	}
	if len(fc) == 0 {
		return nil
	}
	return fc
}

// userConfigPath is the per-user config file, honouring XDG_CONFIG_HOME and
// falling back to ~/.config/draft/config.toml. Empty when no home is known.
func userConfigPath() string {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "draft", "config.toml")
	}
	home, err := userHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "draft", "config.toml")
}

// readFileConfig reads and parses one file. An absent file yields nothing;
// draft runs the same with or without it.
func readFileConfig(path string, warn func(string, ...any)) fileConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return parseFileConfig(data, warn)
}

func merge(dst, src fileConfig) {
	for k, v := range src {
		dst[k] = v
	}
}

// parseFileConfig reads a deliberately small TOML subset: flat "key = value"
// lines, "#" or ";" comments, and blank lines. Section headers ("[table]") are
// skipped so a grouped file still yields its top-level keys. Values may be
// double- or single-quoted; a quoted value is taken verbatim, a bare value is
// trimmed. Keys are lower-cased and underscores folded to hyphens.
func parseFileConfig(data []byte, warn func(string, ...any)) fileConfig {
	fc := fileConfig{}
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			warn("config line %d: expected key = value, got %q; skipping it", i+1, line)
			continue
		}
		key := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(k)), "_", "-")
		val := unquoteValue(strings.TrimSpace(v))
		if key == "" || val == "" {
			continue
		}
		fc[key] = val
	}
	return fc
}

// unquoteValue strips one layer of matching surrounding quotes. An unquoted
// value keeps a trailing inline comment only if it is genuinely part of the
// value; the documented form is quoted, which removes the ambiguity.
func unquoteValue(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}
