// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sebastienrousseau/draft/frontmatter"
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

// site is the publisher identity the [site] section sets.
func (fc fileConfig) site() frontmatter.Site {
	return frontmatter.DefaultSite.Override(func(key string) string { return fc.get("site."+key, "") })
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
// $XDG_CONFIG_HOME/draft (or ~/.config/draft), but only once it is trusted
// (see readProjectConfig). DRAFT_NO_CONFIG disables the layer entirely. A
// missing discovered file is silent; a missing DRAFT_CONFIG is a warning,
// because the user asked for it by name, and naming it is trust enough.
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
		merge(fc, readProjectConfig(filepath.Join(wd, projectConfigName), warn))
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
// skipped so a grouped file still yields its top-level keys, except [site],
// whose keys are kept apart as "site.<key>": a publisher's name is not an
// engine setting, and "name" alone would collide with the next one that is.
// Values may be double- or single-quoted; a quoted value is taken verbatim, a
// bare value is trimmed. Keys are lower-cased and underscores folded to hyphens.
func parseFileConfig(data []byte, warn func(string, ...any)) fileConfig {
	fc := fileConfig{}
	prefix := ""
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if header, ok := sectionHeader(line); ok {
			prefix = sectionPrefix(header)
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
		fc[prefix+key] = val
	}
	return fc
}

// sectionHeader reports whether line opens a section, and returns the header
// without the comment that may follow it: "[site]  # who publishes" is the
// [site] section, as the README's own example writes it.
func sectionHeader(line string) (string, bool) {
	if !strings.HasPrefix(line, "[") {
		return "", false
	}
	if end := strings.IndexByte(line, ']'); end > 0 {
		return line[:end+1], true
	}
	return line, true
}

// sectionPrefix is the key prefix a section header sets: "site." for [site],
// nothing for any other section, whose keys stay top-level.
func sectionPrefix(header string) string {
	if strings.EqualFold(strings.TrimSpace(strings.Trim(header, "[]")), "site") {
		return "site."
	}
	return ""
}

// unquoteValue strips one layer of matching quotes, and any comment after the
// closing one. An unquoted value is kept whole, "#" and all, since a bare URL
// may carry a fragment; the documented form is quoted, which removes the
// ambiguity. A value whose quote never closes is kept as written.
func unquoteValue(v string) string {
	if v == "" || (v[0] != '"' && v[0] != '\'') {
		return v
	}
	end := strings.IndexByte(v[1:], v[0]) + 1
	if end == 0 {
		return v
	}
	if after := strings.TrimSpace(v[end+1:]); after == "" || after[0] == '#' || after[0] == ';' {
		return v[1:end]
	}
	// Not a comment after the first closing quote: the quotes wrap the whole
	// value, as they always have, inner quotes and all.
	if v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}
