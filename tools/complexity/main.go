// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Command complexity holds every Go function to the repository's complexity
// ceilings (cyclomatic 10, cognitive 15, Halstead difficulty 30, 60 lines;
// 500 lines per file). Code already over one is recorded in a baseline and
// may not get worse; anything else over a ceiling fails. An offender that
// improves fails too until the baseline is rewritten, so the recorded
// backlog can only shrink.
//
// It is its own module so its analysis libraries stay out of draft's.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("complexity", flag.ContinueOnError)
	fl.SetOutput(stderr)
	root := fl.String("root", ".", "module root to measure")
	path := fl.String("baseline", "complexity-baseline.txt", "baseline file")
	write := fl.Bool("write", false, "record improvements in the baseline")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	measured, baseline, exists, err := load(*root, *path)
	if err != nil {
		fmt.Fprintln(stderr, "complexity:", err)
		return 2
	}
	regressions, stale := compare(measured, baseline)
	for _, r := range regressions {
		fmt.Fprintln(stderr, r)
	}
	if *write {
		return record(*path, measured, exists && len(regressions) > 0, stdout, stderr)
	}
	for _, s := range stale {
		fmt.Fprintln(stderr, s)
	}
	if len(regressions)+len(stale) > 0 {
		return 1
	}
	fmt.Fprintf(stdout, "complexity: no function or file is worse than %s\n", *path)
	return 0
}

// load measures root and reads the baseline, reporting whether one exists.
func load(root, path string) (measured, baseline Metrics, exists bool, err error) {
	if measured, err = measure(root); err != nil {
		return nil, nil, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return measured, Metrics{}, false, nil
	}
	if err != nil {
		return nil, nil, false, err
	}
	baseline, err = parseBaseline(bytes.NewReader(data))
	return measured, baseline, true, err
}

// record rewrites the baseline from the current measurement, but not over a
// regression: rewriting to absorb one is the thing the baseline must never
// do. With no baseline yet everything is recorded, which is how a repository
// adopts the gate; that first file is reviewed like any other change.
func record(path string, measured Metrics, regressed bool, stdout, stderr io.Writer) int {
	if regressed {
		fmt.Fprintln(stderr, "complexity: not rewriting the baseline over a regression; fix the code instead")
		return 1
	}
	if err := os.WriteFile(path, []byte(formatBaseline(offenders(measured))), 0o644); err != nil {
		fmt.Fprintln(stderr, "complexity:", err)
		return 2
	}
	fmt.Fprintf(stdout, "complexity: wrote %d offender(s) to %s\n", len(offenders(measured)), path)
	return 0
}

// compare returns what is over its ceiling beyond the recorded value, and
// what the baseline records above the current value (an improvement, or a
// unit that no longer exists, not yet written down).
func compare(measured, baseline Metrics) (regressions, stale []string) {
	for _, key := range sortedKeys(measured) {
		for _, k := range sortedKeys(measured[key]) {
			v, allowed := measured[key][k], max(ceilings[k], baseline[key][k])
			if v > allowed {
				regressions = append(regressions, fmt.Sprintf("%s: %s %s is over %s", key, k, num(v), num(allowed)))
			}
		}
	}
	for _, key := range sortedKeys(baseline) {
		for _, k := range sortedKeys(baseline[key]) {
			b, now := baseline[key][k], max(ceilings[k], measured[key][k])
			if b > now {
				stale = append(stale, fmt.Sprintf("%s: %s improved from %s to %s; run `make complexity-baseline` to record it", key, k, num(b), num(measured[key][k])))
			}
		}
	}
	return regressions, stale
}

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
