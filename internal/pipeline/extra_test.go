// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/internal/engine"
)

func TestContinuationBudgetExhausted(t *testing.T) {
	cfg := testConfig(t)
	// Every write stops on a length limit and never ends on punctuation, so the
	// continuation budget is exhausted and validation reports truncation.
	eng := &fakeEngine{name: "fake", writer: func(int) (string, bool) {
		return validArticle(" and it keeps going"), true
	}}
	_, errText, logs := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if errText == "" {
		t.Fatal("expected truncation failure after exhausting continuations")
	}
	if eng.writeCalls < cfg.MaxContinue {
		t.Errorf("expected the continuation loop to run, calls=%d", eng.writeCalls)
	}
	if !hasLog(logs, "length limit") {
		t.Errorf("expected a length-limit log, got %v", logs)
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
