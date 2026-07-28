// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/pipeline"
)

// demoEngine stands in for a backend: deterministic, in-process, no network.
type demoEngine struct{}

func (demoEngine) Name() string { return "demo" }

func (demoEngine) Generate(_ context.Context, req engine.Request) (engine.Result, error) {
	if req.Kind == engine.KindExtract {
		return engine.Result{Text: "CLAIM: It used 5x fewer FLOPs\nSOURCE_QUOTE: \"used 5x fewer FLOPs\"\nTYPE: result\nSTRENGTH: demonstrated\n---"}, nil
	}
	body := strings.Repeat("The grounded result stands on its own and reads plainly. ", 110)
	return engine.Result{Text: "# Router-S Cuts Compute\n\n**One number tells the story.**\n\n" +
		"<aside class=\"post-lead\"><p><strong>TL;DR.</strong> Fewer FLOPs.</p></aside>\n\n" +
		"> **Executive Summary**\n>\n> - It used 5x fewer FLOPs.\n\n## What it shows\n\n" + body + "."}, nil
}

// A Runner turns sources into a finished draft, reporting progress as events.
// Several sources in one Job produce a single merged draft; one Job per source
// produces one draft each.
func ExampleRunner_Run() {
	dir, _ := os.MkdirTemp("", "draft-example-*")
	defer os.RemoveAll(dir)

	src := filepath.Join(dir, "paper.txt")
	_ = os.WriteFile(src, []byte("It used 5x fewer FLOPs than the dense baseline."), 0o644)

	events := make(chan pipeline.Event, 256)
	go func() {
		cfg := config.Config{HomeDir: dir, DraftsDir: dir, MaxContinue: 3}
		pipeline.NewRunner(cfg, []engine.Engine{demoEngine{}}, events).
			Run(context.Background(), pipeline.Job{Sources: []string{src}})
		close(events)
	}()

	for e := range events {
		if done, ok := e.(pipeline.DoneEvent); ok {
			fmt.Println("engine:", done.Engine)
			fmt.Println("final document:", filepath.Base(filepath.Dir(done.OutputPath)))
		}
	}
	// Output:
	// engine: demo
	// final document: final
}
