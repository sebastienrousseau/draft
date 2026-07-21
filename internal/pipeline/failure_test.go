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

func TestNoReadableText(t *testing.T) {
	cfg := testConfig(t)
	bad := filepath.Join(t.TempDir(), "only.xyz")
	os.WriteFile(bad, []byte("x"), 0o644)
	_, errText, _ := drain(t, cfg, []engine.Engine{okEngine("fake")}, Job{Sources: []string{bad}})
	if !strings.Contains(errText, "no readable text") {
		t.Errorf("expected no-readable-text error, got %q", errText)
	}
}

func TestFailureNonArticleRawOnly(t *testing.T) {
	cfg := testConfig(t)
	// Plain prose: fails validation and does not look like an article, so only
	// the raw output is saved (no needs-review copy).
	eng := &fakeEngine{name: "fake", writer: func(int) (string, bool) {
		return "just plain prose with no markdown structure at all and it is short.", false
	}}
	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if !strings.Contains(errText, "Raw output saved") {
		t.Errorf("expected raw output note, got %q", errText)
	}
	if strings.Contains(errText, "Needs-review") {
		t.Error("non-article output should not produce a needs-review copy")
	}
}

func TestFailureArticleLikeGetsReviewCopy(t *testing.T) {
	cfg := testConfig(t)
	eng := &fakeEngine{name: "fake", writer: func(int) (string, bool) {
		return "# Looks Like One\n\n## Section\n\nbut far too short to pass.", false
	}}
	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if !strings.Contains(errText, "Needs-review") {
		t.Errorf("article-like failure should save a needs-review copy, got %q", errText)
	}
}

func TestContinuationEmptyThenTrims(t *testing.T) {
	cfg := testConfig(t)
	eng := &fakeEngine{name: "fake", writer: func(call int) (string, bool) {
		if call == 1 {
			return validArticle(" unfinished"), true // truncated, no closing punctuation
		}
		return "", false // empty continuation -> loop breaks, tail is trimmed
	}}
	done, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("an empty continuation should be rescued by trimming, got: %s", errText)
	}
	if done.OutputPath == "" {
		t.Fatal("expected a saved article after trimming")
	}
	if eng.writeCalls < 2 {
		t.Errorf("continuation should have been attempted, calls=%d", eng.writeCalls)
	}
}

func TestContinuationCallErrors(t *testing.T) {
	cfg := testConfig(t)
	// First write is truncated; the continuation call errors -> loop breaks and
	// the tail is trimmed to the last complete sentence, so the draft still saves
	// rather than being lost over a failed continuation.
	eng := &fakeEngine{name: "fake", errOnWrite: 2, writer: func(int) (string, bool) {
		return validArticle(" unfinished"), true
	}}
	done, errText, logs := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if errText != "" {
		t.Fatalf("a continuation error should be rescued by trimming, got: %s", errText)
	}
	if done.OutputPath == "" {
		t.Fatal("expected a saved article after trimming")
	}
	if !hasLog(logs, "continuation failed") {
		t.Errorf("expected a continuation-failed log, got %v", logs)
	}
}

func TestRetryGenerationErrors(t *testing.T) {
	cfg := testConfig(t)
	cfg.WriteRetries = 1
	// First attempt fails validation; the retry generation call errors out.
	eng := &fakeEngine{name: "fake", errOnWrite: 2, writer: func(int) (string, bool) {
		return "# Bad\n\n## S\n\ntoo short.", false
	}}
	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if !strings.Contains(errText, "generation failed") {
		t.Errorf("expected generation-failed error, got %q", errText)
	}
}

func TestRunOutputDirUnwritable(t *testing.T) {
	cfg := testConfig(t)
	// Point DraftsDir at a regular file so creating the dated subdir fails.
	f := filepath.Join(t.TempDir(), "not-a-dir")
	os.WriteFile(f, []byte("x"), 0o644)
	cfg.DraftsDir = f
	_, errText, _ := drain(t, cfg, []engine.Engine{okEngine("fake")}, Job{Sources: []string{writeSource(t)}})
	if errText == "" {
		t.Error("expected an error when the output dir cannot be created")
	}
}

func TestInitialWriteError(t *testing.T) {
	cfg := testConfig(t)
	// Extraction succeeds but the first write call errors, with no fallback.
	eng := &fakeEngine{name: "solo", errOnWrite: 1, writer: func(int) (string, bool) { return validArticle("."), false }}
	_, errText, _ := drain(t, cfg, []engine.Engine{eng}, Job{Sources: []string{writeSource(t)}})
	if !strings.Contains(errText, "generation failed") {
		t.Errorf("expected generation-failed error, got %q", errText)
	}
}
