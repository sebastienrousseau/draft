// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/internal/engine"
	"github.com/sebastienrousseau/draft/internal/rules"
	"github.com/sebastienrousseau/draft/internal/validate"
)

func TestContinuationBudgetExhaustedTrimsToSentence(t *testing.T) {
	cfg := testConfig(t)
	// The first write is a full but unterminated article; each continuation also
	// stops on a length limit without closing. Once the budget is spent, the tail
	// is trimmed to the last complete sentence, so the draft still saves cleanly
	// instead of failing as truncated (which would force an expensive rewrite).
	eng := &fakeEngine{name: "fake", writer: func(call int) (string, bool) {
		if call == 1 {
			return validArticle(" and the thought trails off with no close"), true
		}
		return " still more unfinished text", true
	}}
	done, errText, logs := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("trim fallback should let the draft save, got error: %s", errText)
	}
	if done.OutputPath == "" {
		t.Fatal("expected a saved article after trimming")
	}
	if eng.writeCalls < cfg.MaxContinue {
		t.Errorf("expected the continuation loop to run, calls=%d", eng.writeCalls)
	}
	if eng.lastWritePred != continuePredictTokens {
		t.Errorf("continuations should use the small cap %d, got %d", continuePredictTokens, eng.lastWritePred)
	}
	if !hasLog(logs, "trimmed a ragged tail") {
		t.Errorf("expected a trim log, got %v", logs)
	}
	data, _ := os.ReadFile(done.OutputPath)
	if !validate.EndsSentence(strings.TrimRight(string(data), " \t\r\n")) {
		t.Error("saved article should end on a complete sentence after trimming")
	}
}

func TestEnforceStyleRemovesBannedTerms(t *testing.T) {
	in := "Furthermore, this robust and seamless approach is paramount. In the realm of scale it will revolutionize things."
	got := enforceStyle(in)
	// No banned word or phrase may survive, and sentence-initial case is kept.
	for _, w := range rules.BannedWords {
		if wordBoundaryHit(strings.ToLower(got), w) {
			t.Errorf("banned word %q survived: %q", w, got)
		}
	}
	for _, p := range rules.BannedPhrases {
		if strings.Contains(strings.ToLower(got), p) {
			t.Errorf("banned phrase %q survived: %q", p, got)
		}
	}
	if !strings.HasPrefix(got, "Also,") {
		t.Errorf("sentence-initial replacement should keep capitalisation: %q", got)
	}
}

func TestEnforceStyleEveryBannedTermHasAReplacement(t *testing.T) {
	// Guards the rules invariant: nothing the validator bans is left uncovered by
	// StyleReplacements, and no replacement is itself banned.
	banned := map[string]bool{}
	for _, w := range append(append([]string{}, rules.BannedWords...), rules.BannedPhrases...) {
		banned[w] = true
		repl, ok := rules.StyleReplacements[w]
		if !ok || strings.TrimSpace(repl) == "" {
			t.Errorf("banned term %q has no style replacement", w)
		}
	}
	for term, repl := range rules.StyleReplacements {
		if banned[strings.ToLower(repl)] {
			t.Errorf("replacement %q for %q is itself banned", repl, term)
		}
	}
}

// wordBoundaryHit reports a whole-word match, mirroring the validator.
func wordBoundaryHit(s, word string) bool {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
	return re.MatchString(s)
}

func TestNormalizeStripsLeakedThesisLabel(t *testing.T) {
	in := "# Title\n\n**Opening thesis paragraph.** The real thesis follows here.\n"
	got := normalizeDraft(in)
	if strings.Contains(strings.ToLower(got), "opening thesis paragraph") {
		t.Errorf("leaked thesis label should be stripped: %q", got)
	}
	if !strings.Contains(got, "The real thesis follows here.") {
		t.Errorf("real thesis should survive the strip: %q", got)
	}
}

func TestNormalizeDropsUnfilledThesisButKeepsRealOne(t *testing.T) {
	// A bare ellipsis/empty bold thesis is a copied placeholder: drop the line.
	for _, ph := range []string{"**...**", "**…**", "****", "**  **"} {
		got := normalizeDraft("# Title\n\n" + ph + "\n\n## Section\n\nbody.")
		if strings.Contains(got, ph) {
			t.Errorf("unfilled thesis %q should be dropped: %q", ph, got)
		}
	}
	// A real bold thesis must be preserved untouched.
	got := normalizeDraft("# Title\n\n**Router-Q cuts FLOPs threefold.**\n\n## S\n\nbody.")
	if !strings.Contains(got, "**Router-Q cuts FLOPs threefold.**") {
		t.Errorf("a real bold thesis must survive: %q", got)
	}
}

func TestTrimToLastSentence(t *testing.T) {
	cases := []struct{ in, want string }{
		{"A full stop here. Then a fragment", "A full stop here."},
		{"Ends cleanly already.", "Ends cleanly already."},
		{"A loss of 3.1 and then it cuts", ""}, // the '.' in 3.1 is not a boundary
		{"no terminator at all", ""},
		{"Quote closes here.” trailing bit", "Quote closes here.”"},
	}
	for _, c := range cases {
		if got := trimToLastSentence(c.in); got != c.want {
			t.Errorf("trimToLastSentence(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestWriteBudgetScalesWithClaims(t *testing.T) {
	// A thin ledger yields a short target near the floor; a dense one is capped
	// at the house maximum. min stays below max and never under the house floor.
	_, fewMax := writeBudget(2)
	_, manyMax := writeBudget(200)
	if fewMax >= manyMax {
		t.Errorf("more claims should allow a longer article: few=%d many=%d", fewMax, manyMax)
	}
	if manyMax != rules.MaxWords {
		t.Errorf("a dense ledger should reach the house maximum, got %d", manyMax)
	}
	for _, c := range []int{0, 1, 6, 20, 60, 500} {
		lo, hi := writeBudget(c)
		if lo < rules.MinWords {
			t.Errorf("claims=%d: min %d below house floor %d", c, lo, rules.MinWords)
		}
		if hi > rules.MaxWords {
			t.Errorf("claims=%d: max %d above house ceiling %d", c, hi, rules.MaxWords)
		}
		if hi < lo+150 {
			t.Errorf("claims=%d: range too narrow: %d–%d", c, lo, hi)
		}
	}
}

func TestWriteNumPredictBoundedByBudgetAndCeiling(t *testing.T) {
	// A small budget produces a small cap; the ceiling always wins when lower.
	if got := writeNumPredict(1000, 6000); got <= 1000 || got >= 6000 {
		t.Errorf("expected a token cap between the word count and the ceiling, got %d", got)
	}
	if got := writeNumPredict(3000, 1500); got != 1500 {
		t.Errorf("ceiling must clamp the cap, got %d", got)
	}
}

func TestWritePassesBoundedNumPredict(t *testing.T) {
	// The write request must carry a positive token cap sized to the budget, not
	// the raw default, so a local model cannot pad toward its ceiling.
	cfg := testConfig(t)
	cfg.PredictLength = 6000 // the real default ceiling
	eng := okEngine("fake")
	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("unexpected failure: %s", errText)
	}
	if eng.lastWritePred <= 0 {
		t.Errorf("write should receive a bounded NumPredict, got %d", eng.lastWritePred)
	}
	if eng.lastWritePred > cfg.PredictLength {
		t.Errorf("NumPredict %d exceeded ceiling %d", eng.lastWritePred, cfg.PredictLength)
	}
}

func TestSkipsUnreadableSource(t *testing.T) {
	cfg := testConfig(t)
	good := writeSource(t)
	bad := filepath.Join(t.TempDir(), "bad.xyz") // unsupported extension
	os.WriteFile(bad, []byte("x"), 0o644)
	done, errText, logs := drain(t, cfg, []engine.Engine{okEngine("fake")}, Job{Sources: []string{bad, good}})
	if errText != "" {
		t.Fatalf("should still succeed with one readable source: %s", errText)
	}
	if done.OutputPath == "" {
		t.Error("expected an article")
	}
	if !hasLog(logs, "skipped") {
		t.Errorf("expected a skip log, got %v", logs)
	}
}

func TestWarningsLogged(t *testing.T) {
	cfg := testConfig(t)
	// A valid article containing a number absent from the claims produces a
	// faithfulness warning, which is logged (not fatal).
	eng := &fakeEngine{name: "fake", writer: func(int) (string, bool) {
		return validArticle(" It later reached 4242 in follow-up trials."), false
	}}
	done, errText, logs := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("warnings should not fail the run: %s", errText)
	}
	if done.OutputPath == "" {
		t.Error("expected a saved article")
	}
	if !hasLog(logs, "review:") {
		t.Errorf("expected a review warning log, got %v", logs)
	}
}

func TestRetrySucceedsSecondAttempt(t *testing.T) {
	cfg := testConfig(t)
	cfg.WriteRetries = 1
	eng := &fakeEngine{name: "fake", writer: func(call int) (string, bool) {
		if call == 1 {
			return "# Bad\n\n## S\n\ntoo short.", false // fails validation
		}
		return validArticle("."), false // retry succeeds
	}}
	done, errText, logs := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("retry should recover: %s", errText)
	}
	if done.OutputPath == "" || !hasLog(logs, "write retry") {
		t.Errorf("expected a retry log and a saved article, logs=%v", logs)
	}
}

func hasLog(logs []string, sub string) bool {
	for _, l := range logs {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

func TestTruncatedButAlreadyComplete(t *testing.T) {
	cfg := testConfig(t)
	// Backend reports Truncated=true but the text already ends on punctuation:
	// continueGeneration should break immediately without another call.
	eng := &fakeEngine{name: "fake", writer: func(int) (string, bool) { return validArticle("."), true }}
	done, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("should succeed without a continuation: %s", errText)
	}
	if eng.writeCalls != 1 {
		t.Errorf("expected exactly one write call, got %d", eng.writeCalls)
	}
	if done.OutputPath == "" {
		t.Error("expected a saved draft")
	}
}
