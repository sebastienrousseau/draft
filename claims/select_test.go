// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"fmt"
	"strings"
	"testing"
)

func rec(i int, typ string) Record {
	return Record{Claim: fmt.Sprintf("claim %03d", i), SourceQuote: fmt.Sprintf("quote %03d", i), Type: typ, Strength: "demonstrated"}
}

// indexes returns each selected record's position in the full ledger.
func indexes(t *testing.T, all, got []Record) []int {
	t.Helper()
	pos := map[string]int{}
	for i, r := range all {
		pos[r.Claim] = i
	}
	out := make([]int, len(got))
	for i, r := range got {
		p, ok := pos[r.Claim]
		if !ok {
			t.Fatalf("selected a record not in the ledger: %+v", r)
		}
		out[i] = p
	}
	return out
}

func countType(rs []Record, typ string) int {
	n := 0
	for _, r := range rs {
		if r.Type == typ {
			n++
		}
	}
	return n
}

// A ledger that fits is passed through whole and in order, as before.
func TestSelectForPromptKeepsALedgerThatFits(t *testing.T) {
	all := []Record{rec(0, "result"), rec(1, "limitation"), rec(2, "metric")}
	got := SelectForPrompt(all, 3, 100000)
	if len(got) != 3 || got[0] != all[0] || got[1] != all[1] || got[2] != all[2] {
		t.Errorf("a ledger that fits changed: %+v", got)
	}
}

// The writer used to get the first records in source order, which on a long
// paper is the abstract and introduction, with the limitations, usually near
// the end, left out. Selection now spans the ledger and reserves a fifth of
// the slots for limitations.
func TestSelectForPromptSpansTheLedgerAndReservesLimitations(t *testing.T) {
	var all []Record
	for i := 0; i < 100; i++ {
		all = append(all, rec(i, "result"))
	}
	for i := 100; i < 120; i++ {
		all = append(all, rec(i, "limitation"))
	}
	got := SelectForPrompt(all, 45, 1000000)
	if len(got) != 45 {
		t.Fatalf("selected %d records, want 45", len(got))
	}
	if n := countType(got, "limitation"); n != 9 {
		t.Errorf("selected %d limitations, want 9 (a fifth of 45)", n)
	}
	idx := indexes(t, all, got)
	for i := 1; i < len(idx); i++ {
		if idx[i] <= idx[i-1] {
			t.Fatalf("selection is not in source order: %v", idx)
		}
	}
	if idx[0] > 5 || idx[35] < 90 {
		t.Errorf("results are not spread across the ledger: first %d, last result %d", idx[0], idx[35])
	}
	// Evenly spaced, centred picks: 36 of 100 results, 9 of 20 limitations.
	if idx[0] != 1 || idx[1] != 4 || idx[36] != 101 || idx[44] != 118 {
		t.Errorf("picks are not evenly spaced: %v", idx)
	}
}

// Fewer limitations than the reserve: every one is kept.
func TestSelectForPromptKeepsEveryLimitationUnderTheReserve(t *testing.T) {
	var all []Record
	for i := 0; i < 100; i++ {
		typ := "result"
		if i == 50 || i == 99 {
			typ = "limitation"
		}
		all = append(all, rec(i, typ))
	}
	got := SelectForPrompt(all, 45, 1000000)
	if len(got) != 45 || countType(got, "limitation") != 2 {
		t.Errorf("got %d records with %d limitations, want 45 with 2", len(got), countType(got, "limitation"))
	}
}

// Slots the other records cannot fill go to more limitations, and a tiny cap
// still reserves at least one slot for them.
func TestSelectForPromptFillsSpareSlotsWithLimitations(t *testing.T) {
	var all []Record
	for i := 0; i < 5; i++ {
		all = append(all, rec(i, "result"))
	}
	for i := 5; i < 35; i++ {
		all = append(all, rec(i, "limitation"))
	}
	got := SelectForPrompt(all, 10, 1000000)
	if len(got) != 10 || countType(got, "result") != 5 || countType(got, "limitation") != 5 {
		t.Errorf("got %d records, %d results, %d limitations; want 10, 5, 5", len(got), countType(got, "result"), countType(got, "limitation"))
	}
	one := SelectForPrompt([]Record{rec(0, "result"), rec(1, "limitation"), rec(2, "result")}, 1, 1000000)
	if len(one) != 1 || one[0].Type != "limitation" {
		t.Errorf("a one-slot cap = %+v, want the limitation", one)
	}
	if got := SelectForPrompt(all, 0, 1000000); got != nil {
		t.Errorf("a zero cap selected %d records", len(got))
	}
}

// The character budget drops other records before limitations, and a record
// that does not fit is skipped without ending the selection.
func TestSelectForPromptBudgetsLimitationsFirst(t *testing.T) {
	long := Record{Claim: strings.Repeat("x", 400), SourceQuote: "a long quote", Type: "result", Strength: "demonstrated"}
	all := []Record{long, rec(1, "result"), rec(2, "limitation"), rec(3, "result")}
	budget := promptHeaderLen(len(all)) + blockLen(all[2]) + blockLen(all[1]) + blockLen(all[3])
	got := SelectForPrompt(all, 4, budget)
	idx := indexes(t, all, got)
	if len(idx) != 3 || idx[0] != 1 || idx[1] != 2 || idx[2] != 3 {
		t.Errorf("budgeted selection = %v, want [1 2 3] (the long record skipped)", idx)
	}
	tight := SelectForPrompt(all, 4, promptHeaderLen(len(all))+blockLen(all[2]))
	if len(tight) != 1 || tight[0] != all[2] {
		t.Errorf("a budget for one record kept %+v, want the limitation", tight)
	}
}

// RenderPromptLedger renders exactly the selection, and says how much of the
// ledger it holds.
func TestRenderPromptLedgerRendersTheSelection(t *testing.T) {
	var all []Record
	for i := 0; i < 60; i++ {
		all = append(all, rec(i, "result"))
	}
	all = append(all, rec(60, "limitation"))
	out := RenderPromptLedger(all, 45, 1000000)
	if !strings.Contains(out, "Included records: 45 of 61") {
		t.Errorf("count line wrong:\n%.200s", out)
	}
	for _, r := range SelectForPrompt(all, 45, 1000000) {
		if !strings.Contains(out, "CLAIM: "+r.Claim+"\n") {
			t.Errorf("selected %q is missing from the rendered ledger", r.Claim)
		}
	}
	if !strings.Contains(out, "CLAIM: claim 060\n") {
		t.Error("the limitation at the end of the ledger did not reach the writer")
	}
	if got := strings.Count(out, "CLAIM: "); got != 45 {
		t.Errorf("rendered %d records, want 45", got)
	}
}
