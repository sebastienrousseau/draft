// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/pipeline"
)

// escapeEngine fails extraction with an error carrying terminal escapes, as a
// provider's own output can.
type escapeEngine struct{}

const escapePayload = "\x1b]0;pwned\x07\x1b[2J\rEVIL\u009b31m"

func (escapeEngine) Name() string { return "stub" }
func (escapeEngine) Generate(_ context.Context, req engine.Request) (engine.Result, error) {
	if req.Kind == engine.KindExtract {
		return engine.Result{}, errors.New("provider said " + escapePayload)
	}
	return stubEngine{}.Generate(context.Background(), req)
}

// Headless progress goes to stderr, usually a terminal. Warnings and errors
// that carry a provider's or model's text must reach it without their control
// sequences, while the --json record on stdout keeps the text as it was.
func TestHeadlessStderrCarriesNoControlSequences(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(context.Context, config.Config, *pipeline.Runner, []pipeline.Job, io.Writer, io.Writer) int
	}{
		{"plain", runHeadless},
		{"json", runHeadlessJSON},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, full := tmpSource(t, "paper.txt")
			runner := pipeline.NewRunner(cfg, []engine.Engine{escapeEngine{}}, nil)
			var stdout, stderr strings.Builder
			if tc.run(context.Background(), cfg, runner, []pipeline.Job{{Sources: []string{full}}}, &stdout, &stderr) == 0 {
				t.Fatal("the job should fail when every extraction fails")
			}
			if !strings.Contains(stderr.String(), "provider said") {
				t.Fatalf("the warning was not printed:\n%q", stderr.String())
			}
			if strings.ContainsAny(stderr.String(), "\x1b\x07\r\u009b") {
				t.Errorf("stderr carries a control sequence: %q", stderr.String())
			}
			if tc.name == "json" {
				var rec struct{ Warnings []string }
				if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &rec); err != nil {
					t.Fatal(err)
				}
				if len(rec.Warnings) == 0 || !strings.Contains(rec.Warnings[0], escapePayload) {
					t.Errorf("the JSON record should keep the warning as it was: %q", rec.Warnings)
				}
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

// A writer reports the bytes it was given, not the shorter cleaned text, so a
// caller never sees a short write; and it passes a failure through.
func TestTerminalWriterReportsTheCallersLength(t *testing.T) {
	var b strings.Builder
	in := []byte("ok\x1b[2J\n")
	if n, err := (terminalWriter{&b}).Write(in); n != len(in) || err != nil {
		t.Errorf("Write = %d, %v; want %d, nil", n, err, len(in))
	}
	if b.String() != "ok[2J\n" {
		t.Errorf("wrote %q", b.String())
	}
	if _, err := (terminalWriter{failingWriter{}}).Write(in); err == nil {
		t.Error("a failing writer's error was swallowed")
	}
}
