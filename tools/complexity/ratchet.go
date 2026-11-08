// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

// Checking the code against the branch's own baseline cannot catch the
// baseline itself being loosened: edit a value up, or delete the file and
// rerun -write, and the branch agrees with itself. -compare-baseline holds
// the branch's baseline to the base branch's instead, so an entry can only
// disappear or go down, and a rename has to say where its allowance came
// from.

// runRatchet compares the head baseline with the base branch's.
func runRatchet(basePath, headPath string, stdout, stderr io.Writer) int {
	base, _, err := readBaseline(basePath)
	if err != nil {
		fmt.Fprintln(stderr, "complexity:", err)
		return 2
	}
	head, was, err := readBaseline(headPath)
	if err != nil {
		fmt.Fprintln(stderr, "complexity:", err)
		return 2
	}
	violations := ratchet(base, head, was)
	for _, v := range violations {
		fmt.Fprintln(stderr, v)
	}
	if len(violations) > 0 {
		return 1
	}
	fmt.Fprintf(stdout, "complexity: %s adds no entry to %s and raises no value\n", headPath, basePath)
	return 0
}

// ratchet returns every way head allows more than base: an entry base does
// not have and no was= accounts for, or a value above what base allowed.
func ratchet(base, head Metrics, was Renames) []string {
	var out []string
	claimed := map[string]string{}
	for _, key := range sortedKeys(head) {
		ref, why := allowance(base, head, was, claimed, key)
		if why != "" {
			out = append(out, why)
			continue
		}
		for _, m := range sortedKeys(head[key]) {
			if v, limit := head[key][m], allowed(ref, m); v > limit {
				out = append(out, fmt.Sprintf("%s: %s %s is over the base branch's %s", key, m, num(v), num(limit)))
			}
		}
	}
	return out
}

// allowed is what ref permits for metric m. An entry records only what is
// over a ceiling, so a metric it leaves out was held to the ceiling.
func allowed(ref map[string]float64, m string) float64 {
	if v, ok := ref[m]; ok {
		return v
	}
	return ceilings[m]
}

// allowance is the base entry key is held to: its own, or the one its was=
// names. That one must be in base, gone from head, and claimed only once,
// so one offender's allowance cannot be moved and kept, or handed to two.
func allowance(base, head Metrics, was Renames, claimed map[string]string, key string) (map[string]float64, string) {
	if ref, ok := base[key]; ok {
		return ref, ""
	}
	from, ok := was[key]
	switch {
	case !ok:
		return nil, key + ": new entry; fix it, or if it was renamed or moved, record that with -rename"
	case base[from] == nil:
		return nil, fmt.Sprintf("%s: was=%s is not in the base branch's baseline", key, encodeKey(from))
	case head[from] != nil:
		return nil, fmt.Sprintf("%s: was=%s is still recorded, so its allowance cannot move", key, encodeKey(from))
	case claimed[from] != "":
		return nil, fmt.Sprintf("%s: was=%s is already claimed by %s", key, encodeKey(from), claimed[from])
	}
	claimed[from] = key
	return base[from], ""
}

// renameFlags maps each -rename's new key to its old one.
type renameFlags map[string]string

func (r renameFlags) String() string { return "" }

func (r renameFlags) Set(v string) error {
	o, n, ok := strings.Cut(v, "=")
	if !ok || !strings.Contains(o, ":") || !strings.Contains(n, ":") {
		return errors.New("want old=new, each path:name")
	}
	r[decodeKey(n)] = decodeKey(o)
	return nil
}

// apply moves each renamed entry's allowance to its new key, so the run
// holds the new name to the old limits, and marks it was= for the check
// against the base branch. A rename must name an entry the baseline holds,
// whose code is no longer over a ceiling under that name, onto one that is.
func (r renameFlags) apply(st *state) error {
	off := offenders(st.measured)
	for _, n := range sortedKeys(r) {
		o := r[n]
		switch {
		case st.baseline[o] == nil:
			return fmt.Errorf("-rename: %s is not in the baseline", encodeKey(o))
		case off[o] != nil:
			return fmt.Errorf("-rename: %s is still over a ceiling, so its entry stays", encodeKey(o))
		case off[n] == nil:
			return fmt.Errorf("-rename: %s is over no ceiling, so there is nothing to carry over", encodeKey(n))
		}
		st.baseline[n], st.was[n] = st.baseline[o], o
		delete(st.baseline, o)
	}
	return nil
}
