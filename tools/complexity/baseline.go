// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

const baselineHeader = `# SPDX-FileCopyrightText: 2026 Sebastien Rousseau
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# Functions and files already over a complexity ceiling, with the value each
# may not exceed. Written by ` + "`make complexity-baseline`" + `, which refuses
# while anything is worse than recorded, so this file only ever shrinks.
# Format: <path> <function or (file)> <metric>=<value>...
`

// parseBaseline reads the "path name metric=value..." lines written by
// writeBaseline, skipping comments and blank lines.
func parseBaseline(r io.Reader) (Metrics, error) {
	b := Metrics{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return nil, fmt.Errorf("baseline line %d: want <path> <name> <metric>=<value>", n)
		}
		vals, err := parseValues(fields[2:])
		if err != nil {
			return nil, fmt.Errorf("baseline line %d: %w", n, err)
		}
		b[fields[0]+" "+fields[1]] = vals
	}
	return b, sc.Err()
}

func parseValues(fields []string) (map[string]float64, error) {
	vals := map[string]float64{}
	for _, f := range fields {
		k, v, ok := strings.Cut(f, "=")
		if _, known := ceilings[k]; !ok || !known {
			return nil, fmt.Errorf("bad metric %q", f)
		}
		x, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("bad value %q", f)
		}
		vals[k] = x
	}
	return vals, nil
}

// offenders keeps only the metrics over their ceiling, the part of a
// measurement a baseline records.
func offenders(m Metrics) Metrics {
	out := Metrics{}
	for key, vals := range m {
		for k, v := range vals {
			if v > ceilings[k] {
				if out[key] == nil {
					out[key] = map[string]float64{}
				}
				out[key][k] = v
			}
		}
	}
	return out
}

// formatBaseline renders b as the baseline file, sorted so a rewrite diffs
// only where something changed.
func formatBaseline(b Metrics) string {
	var buf strings.Builder
	buf.WriteString(baselineHeader)
	for _, key := range sortedKeys(b) {
		buf.WriteString(key)
		for _, k := range sortedKeys(b[key]) {
			fmt.Fprintf(&buf, " %s=%s", k, strconv.FormatFloat(b[key][k], 'f', -1, 64))
		}
		buf.WriteByte('\n')
	}
	return buf.String()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
