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
# Format: <path> <function or (file)> <metric>=<value>... [was=<path>:<name>]
# was= marks an entry renamed or moved from the named one, for the check
# against the base branch's baseline; record it with -rename, not by hand.
`

// Renames maps a baseline key to the key it was recorded under before a
// rename or move, so the check against the base branch can carry the old
// allowance over instead of reading the entry as new.
type Renames map[string]string

// parseBaseline reads the "path name metric=value... [was=path:name]" lines
// written by formatBaseline, skipping comments and blank lines.
func parseBaseline(r io.Reader) (Metrics, Renames, error) {
	b, was := Metrics{}, Renames{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return nil, nil, fmt.Errorf("baseline line %d: want <path> <name> <metric>=<value>", n)
		}
		key, metrics := fields[0]+" "+fields[1], fields[2:]
		if last := metrics[len(metrics)-1]; strings.HasPrefix(last, "was=") {
			was[key], metrics = decodeKey(strings.TrimPrefix(last, "was=")), metrics[:len(metrics)-1]
		}
		vals, err := parseValues(metrics)
		if err != nil {
			return nil, nil, fmt.Errorf("baseline line %d: %w", n, err)
		}
		b[key] = vals
	}
	return b, was, sc.Err()
}

// encodeKey writes a "path name" key as one field, "path:name"; decodeKey
// reverses it. A Go name never holds a colon, so the last one is the split.
func encodeKey(key string) string { return strings.Replace(key, " ", ":", 1) }

func decodeKey(field string) string {
	if i := strings.LastIndex(field, ":"); i >= 0 {
		return field[:i] + " " + field[i+1:]
	}
	return field
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
func formatBaseline(b Metrics, was Renames) string {
	var buf strings.Builder
	buf.WriteString(baselineHeader)
	for _, key := range sortedKeys(b) {
		buf.WriteString(key)
		for _, k := range sortedKeys(b[key]) {
			fmt.Fprintf(&buf, " %s=%s", k, strconv.FormatFloat(b[key][k], 'f', -1, 64))
		}
		if from, ok := was[key]; ok {
			buf.WriteString(" was=" + encodeKey(from))
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
