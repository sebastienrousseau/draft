// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Logic written as a package-level function literal used to escape every
// ceiling. It is measured now, under the variable's name.
func TestMeasureIncludesPackageLevelFuncLiterals(t *testing.T) {
	dir := t.TempDir()
	lit := strings.Replace(busy("x", 12), "func x(", "func(", 1)
	write(t, dir, "v.go", "package v\n\nvar (\n\tplain = 1\n\tHook, other = "+strings.TrimSuffix(lit, "\n")+", 2\n)\n\nconst c = 1\n")
	m, err := measure(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := sortedKeys(m); strings.Join(got, ",") != "v.go (file),v.go Hook" {
		t.Fatalf("measured units = %v", got)
	}
	if m["v.go Hook"][cyclomatic] != 13 || m["v.go Hook"][lines] != 39 {
		t.Errorf("Hook = %v", m["v.go Hook"])
	}
}

// Two init functions in a file used to share one key, so the second
// overwrote the first and only one was held to the ceilings.
func TestRepeatedInitsAreEachMeasured(t *testing.T) {
	dir := t.TempDir()
	busyInit := strings.Replace(strings.Replace(busy("init", 12), "(x int) int", "()", 1), "\treturn x\n", "", 1)
	write(t, dir, "i.go", "package i\n\nvar x int\n\n"+busyInit+"\nfunc init() {}\n")
	m, err := measure(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m["i.go init"][cyclomatic] != 13 || m["i.go init#2"][cyclomatic] != 1 {
		t.Errorf("inits = %v / %v, want the busy one first and the empty one as #2", m["i.go init"], m["i.go init#2"])
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func ratchetRun(t *testing.T, base, head string) (int, string) {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "base.txt", base)
	write(t, dir, "head.txt", head)
	var out, errs bytes.Buffer
	code := run([]string{"-compare-baseline", filepath.Join(dir, "base.txt"), "-baseline", filepath.Join(dir, "head.txt")}, &out, &errs)
	return code, out.String() + errs.String()
}

// The finding: CI held the code to the branch's own baseline, so raising a
// value in it, or deleting it and rerunning -write, passed. Against the base
// branch's baseline, only removing or lowering passes.
func TestCompareBaselineAllowsOnlyShrinking(t *testing.T) {
	const base = "a.go F cyclomatic=13 lines=70\nb.go G cognitive=20\n"
	for _, tc := range []struct {
		name, head string
		code       int
		want       string
	}{
		{"unchanged", base, 0, "adds no entry"},
		{"lowered and removed", "a.go F cyclomatic=12\n", 0, "adds no entry"},
		{"raised", "a.go F cyclomatic=14 lines=70\nb.go G cognitive=20\n", 1, "a.go F: cyclomatic 14 is over the base branch's 13"},
		{"new metric", "a.go F cyclomatic=13 halstead=31 lines=70\n", 1, "halstead 31 is over the base branch's 30"},
		{"new entry", base + "c.go H lines=61\n", 1, "c.go H: new entry"},
		{"rename", "a.go F cyclomatic=13 lines=70\nc.go G2 cognitive=19 was=b.go:G\n", 0, "adds no entry"},
		{"rename raises", "c.go G2 cognitive=21 was=b.go:G\n", 1, "cognitive 21 is over the base branch's 20"},
		{"rename from nothing", "c.go G2 cognitive=19 was=z.go:Z\n", 1, "was=z.go:Z is not in the base branch's baseline"},
		{"rename but kept", base + "c.go G2 cognitive=19 was=b.go:G\n", 1, "was=b.go:G is still recorded"},
		{"rename twice", "c.go G2 cognitive=19 was=b.go:G\nc.go G3 cognitive=19 was=b.go:G\n", 1, "was=b.go:G is already claimed by c.go G2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, out := ratchetRun(t, base, tc.head)
			if code != tc.code || !strings.Contains(out, tc.want) {
				t.Errorf("exit %d, output %q; want %d and %q", code, out, tc.code, tc.want)
			}
		})
	}
}

func TestCompareBaselineReportsUnreadableFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "ok.txt", "a.go F lines=70\n")
	missing := filepath.Join(dir, "missing.txt")
	for _, args := range [][]string{{missing, filepath.Join(dir, "ok.txt")}, {filepath.Join(dir, "ok.txt"), missing}} {
		var out, errs bytes.Buffer
		if code := run([]string{"-compare-baseline", args[0], "-baseline", args[1]}, &out, &errs); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
}

// Renaming an offender used to need a hand edit of the baseline, which looks
// exactly like loosening it. -rename carries the old allowance over and marks
// the entry was=, which the base-branch check accepts once.
func TestRenameCarriesTheAllowanceOver(t *testing.T) {
	dir := tree(t, 12)
	gate(t, dir, "-write")
	baseline := filepath.Join(dir, "baseline.txt")
	mainCopy := readFile(t, baseline)

	write(t, dir, "a/a.go", "package a\n\n"+busy("Renamed", 12)+"\nfunc Small() {}\n")
	if code, out := gate(t, dir); code != 1 || !strings.Contains(out, "a/a.go Renamed: cyclomatic 13 is over 10") {
		t.Fatalf("a rename without -rename must fail: %d %q", code, out)
	}
	if code, out := gate(t, dir, "-write", "-rename", "a/a.go:Busy=a/a.go:Renamed"); code != 0 {
		t.Fatalf("-write -rename: %d %q", code, out)
	}
	if got := readFile(t, baseline); !strings.Contains(got, "a/a.go Renamed cyclomatic=13 ") || !strings.HasSuffix(got, " was=a/a.go:Busy\n") || strings.Contains(got, " Busy ") {
		t.Fatalf("baseline after rename:\n%s", got)
	}
	if code, out := gate(t, dir); code != 0 {
		t.Fatalf("renamed entry must pass: %d %q", code, out)
	}
	write(t, dir, "main.txt", mainCopy)
	var out, errs bytes.Buffer
	if code := run([]string{"-compare-baseline", filepath.Join(dir, "main.txt"), "-baseline", baseline}, &out, &errs); code != 0 {
		t.Fatalf("against main: %d %q", code, errs.String())
	}

	// Recording an improvement later on the branch keeps the mark.
	write(t, dir, "a/a.go", "package a\n\n"+busy("Renamed", 11)+"\nfunc Small() {}\n")
	gate(t, dir, "-write")
	if got := readFile(t, baseline); !strings.Contains(got, "a/a.go Renamed cyclomatic=12 ") || !strings.HasSuffix(got, " was=a/a.go:Busy\n") {
		t.Fatalf("an improvement dropped the rename:\n%s", got)
	}
}

func TestRenameRefusesWhatItCannotCarry(t *testing.T) {
	dir := tree(t, 12)
	gate(t, dir, "-write")
	write(t, dir, "a/b.go", "package a\n\n"+busy("Other", 12))
	for _, tc := range []struct{ rename, want string }{
		{"a/a.go:Nope=a/b.go:Other", "a/a.go:Nope is not in the baseline"},
		{"a/a.go:Busy=a/b.go:Other", "a/a.go:Busy is still over a ceiling"},
		{"bad", "want old=new"},
	} {
		code, out := gate(t, dir, "-rename", tc.rename)
		if code != 2 || !strings.Contains(out, tc.want) {
			t.Errorf("-rename %s: exit %d, %q; want 2 and %q", tc.rename, code, out, tc.want)
		}
	}
	write(t, dir, "a/a.go", "package a\n\nfunc Small() {}\n")
	if code, out := gate(t, dir, "-rename", "a/a.go:Busy=a/a.go:Small"); code != 2 || !strings.Contains(out, "a/a.go:Small is over no ceiling") {
		t.Errorf("rename onto a small function: %d %q", code, out)
	}
}

func TestKeyEncoding(t *testing.T) {
	if got := encodeKey("a/b.go T.M"); got != "a/b.go:T.M" {
		t.Errorf("encodeKey = %q", got)
	}
	if got := decodeKey("a/b.go:T.M"); got != "a/b.go T.M" {
		t.Errorf("decodeKey = %q", got)
	}
	if got := decodeKey("plain"); got != "plain" {
		t.Errorf("decodeKey without a colon = %q", got)
	}
}
