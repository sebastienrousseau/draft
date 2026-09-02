// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/pipeline"
)

// A misspelled provider name used to degrade to Ollama in silence, producing a
// local-model draft the user believed came from Claude. It must now be a clean
// usage failure.
func TestRunRejectsUnknownEngine(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"--engine", "claud", "some-source.pdf"}, &out, &errb)

	if code != 2 {
		t.Errorf("exit code = %d, want 2 for a usage error", code)
	}
	msg := errb.String()
	if !strings.Contains(msg, "claud") {
		t.Errorf("stderr should name the offending value: %q", msg)
	}
	if !strings.Contains(msg, "claude") {
		t.Errorf("stderr should list the valid names so the user can self-correct: %q", msg)
	}
	if out.Len() != 0 {
		t.Errorf("nothing should reach stdout on a usage error, got %q", out.String())
	}
}

func TestRunAcceptsKnownEngine(t *testing.T) {
	var out, errb bytes.Buffer
	// A valid engine with a missing source fails at resolution (1), not
	// validation (2) — which proves validation let it through.
	if code := run([]string{"--engine", "claude", "/no/such/file.pdf"}, &out, &errb); code != 1 {
		t.Errorf("exit code = %d, want 1 (source resolution, not engine validation)", code)
	}
}

// Recovered configuration problems must be reported, not applied silently.
func TestRunReportsConfigWarnings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DRAFT_NUM_CTX", "3") // below the floor

	var out, errb bytes.Buffer
	run([]string{"--engine", "ollama", "/no/such/file.pdf"}, &out, &errb)

	if !strings.Contains(errb.String(), "DRAFT_NUM_CTX") {
		t.Errorf("an out-of-range tunable must be reported, got %q", errb.String())
	}
	if !strings.Contains(errb.String(), "warning") {
		t.Errorf("warnings should be labelled as such, got %q", errb.String())
	}
}

func TestPhaseMillis(t *testing.T) {
	if got := phaseMillis(nil); got != nil {
		t.Errorf("phaseMillis(nil) = %v, want nil so the JSON field is omitted", got)
	}
	got := phaseMillis([]pipeline.PhaseTiming{
		{Index: 0, Name: "Resolve source", Status: "done", Duration: 1500000}, // 1.5ms
		{Index: 1, Name: "Read and section", Status: "done", Duration: 0},
	})
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got["Resolve source"] != 1 {
		t.Errorf("Resolve source = %d ms, want 1", got["Resolve source"])
	}
	if _, ok := got["Read and section"]; !ok {
		t.Error("a zero-duration phase should still appear")
	}
}

// The completion list is derived from the help table, so a flag added to one
// cannot go missing from the other. The old hand-kept copy carried that
// promise in a comment and had fallen four flags behind.
func TestCompletionFlagsCoverEveryDocumentedFlag(t *testing.T) {
	documented := map[string]bool{}
	for _, f := range flagHelp {
		for _, part := range strings.Split(f[0], ",") {
			name, _, _ := strings.Cut(strings.TrimSpace(part), " ")
			if strings.HasPrefix(name, "--") {
				documented[name] = true
			}
		}
	}
	offered := map[string]bool{}
	for _, f := range completionFlags {
		offered[f] = true
	}
	for name := range documented {
		if !offered[name] {
			t.Errorf("%s is documented but not offered by completion", name)
		}
	}
	for name := range offered {
		if !documented[name] {
			t.Errorf("%s is offered by completion but not documented", name)
		}
	}
}

// Every flag the help text documents must actually be registered, or the help
// promises something the parser rejects.
func TestEveryDocumentedFlagIsRegistered(t *testing.T) {
	var out, errb strings.Builder
	for _, f := range flagHelp {
		name, _, _ := strings.Cut(f[0], " ")
		if !strings.HasPrefix(name, "--") || name == "--help" {
			continue
		}
		out.Reset()
		errb.Reset()
		// An unknown flag exits 2 with "flag provided but not defined".
		run([]string{name + "=x"}, &out, &errb)
		if strings.Contains(errb.String(), "not defined") {
			t.Errorf("%s is documented but not registered", name)
		}
	}
}
