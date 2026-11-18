// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// deprecatedEnv parses but is kept out of --help, as the README's
// deprecation window promises. The README still documents it.
var deprecatedEnv = map[string]bool{"DRAFT_CLAUDE_MODEL": true}

// The help ENVIRONMENT list and the README table are written by hand, and
// they had drifted: eleven variables config reads were missing from --help.
// Rather than trust a second copy to stay in step, read the names straight
// from config's source and require each to be documented in every place.
// The manpage prints the help list, so checking it pins that derivation.
func TestEveryConfigVariableIsDocumented(t *testing.T) {
	names := configEnvNames(t)
	if len(names) < 20 {
		t.Fatalf("found only %d variables in config; the scan is broken", len(names))
	}
	var help, man strings.Builder
	usage(&help)
	if err := writeManPage(&man, "test", time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if !deprecatedEnv[name] && !regexp.MustCompile(`\b`+name+`\b`).MatchString(help.String()) {
			t.Errorf("%s is read by config but missing from --help", name)
		}
		if !deprecatedEnv[name] && !strings.Contains(man.String(), name) {
			t.Errorf("%s is read by config but missing from the manpage", name)
		}
		if !strings.Contains(string(readme), "`"+name+"`") {
			t.Errorf("%s is read by config but missing from README.md", name)
		}
	}
}

// configEnvNames lists every DRAFT_* name quoted in config's non-test source.
func configEnvNames(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "config", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	pat := regexp.MustCompile(`"(DRAFT_[A-Z0-9_]+)"`)
	seen := map[string]bool{}
	var names []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range pat.FindAllStringSubmatch(string(src), -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				names = append(names, m[1])
			}
		}
	}
	return names
}
