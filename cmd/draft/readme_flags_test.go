// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// undocumentedFlags parse but stay out of the README's flag table: the
// deprecated alias, which the README covers in its deprecation note instead.
var undocumentedFlags = map[string]bool{"claude-model": true}

// The README's flag table is written by hand, and it had drifted: five
// flags the CLI accepts (--reader, --style, --second-gate, --trust-config,
// --man) were missing from it. Read the names from the flag set itself and
// require a row for each, so a new flag cannot ship undocumented.
func TestEveryFlagIsInTheReadmeTable(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	rows := readmeFlagRows(string(readme))
	if len(rows) < 20 {
		t.Fatalf("found only %d flag rows in the README; the scan is broken", len(rows))
	}
	var o options
	fs := newFlagSet(&o, io.Discard)
	fs.VisitAll(func(f *flag.Flag) {
		if !undocumentedFlags[f.Name] && !rows[f.Name] {
			t.Errorf("--%s is accepted by the CLI but has no row in the README flag table", f.Name)
		}
	})
	for name := range rows {
		if fs.Lookup(name) == nil {
			t.Errorf("the README flag table documents --%s, which the CLI does not accept", name)
		}
	}
}

// readmeFlagRows returns the flag names that begin a row of a Markdown table
// in the README: lines of the form "| `--name ...` | description |".
func readmeFlagRows(readme string) map[string]bool {
	re := regexp.MustCompile("(?m)^\\| `--([a-z][a-z-]*)")
	rows := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(readme, -1) {
		rows[m[1]] = true
	}
	return rows
}
