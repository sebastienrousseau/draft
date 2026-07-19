// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Command pipeline runs the full drafting pipeline end to end against an
// in-process demo engine — no network, no LLM, no session CLI. It shows how the
// Runner streams progress events and where it writes the finished article.
//
// Run it with:
//
//	go run ./examples/pipeline
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sebastienrousseau/draft/internal/config"
	"github.com/sebastienrousseau/draft/internal/engine"
	"github.com/sebastienrousseau/draft/internal/pipeline"
)

// demoEngine is a deterministic engine.Engine: it returns a fixed claim record
// for extraction and a valid grounded article for writing.
type demoEngine struct{}

func (demoEngine) Name() string { return "demo" }

func (demoEngine) Generate(_ context.Context, req engine.Request) (engine.Result, error) {
	if req.Kind == engine.KindExtract {
		return engine.Result{Text: "CLAIM: Router-S used 5x fewer FLOPs\nSOURCE_QUOTE: \"used 5x fewer FLOPs\"\nTYPE: result\nSTRENGTH: demonstrated\n---"}, nil
	}
	body := strings.Repeat("The grounded result stands on its own and reads plainly. ", 110)
	return engine.Result{Text: "# Router-S Cuts Compute\n\n**One number tells the story.**\n\n" +
		"<aside class=\"post-lead\"><p><strong>TL;DR.</strong> Fewer FLOPs.</p></aside>\n\n" +
		"> **Executive Summary**\n>\n> - Router-S used 5x fewer FLOPs.\n\n## What it shows\n\n" + body + "."}, nil
}

func main() {
	dir, err := os.MkdirTemp("", "draft-example-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	src := filepath.Join(dir, "router-s.txt")
	_ = os.WriteFile(src, []byte("Router-S used 5x fewer FLOPs than the dense baseline."), 0o644)

	cfg := config.Config{HomeDir: dir, DraftsDir: dir, MaxContinue: 3}
	events := make(chan pipeline.Event, 256)
	go func() {
		pipeline.NewRunner(cfg, []engine.Engine{demoEngine{}}, events).
			Run(context.Background(), pipeline.Job{Sources: []string{src}})
		close(events)
	}()

	for e := range events {
		switch ev := e.(type) {
		case pipeline.LogEvent:
			fmt.Println("·", string(ev))
		case pipeline.DoneEvent:
			fmt.Printf("\n✓ %d words via %s\n  %s\n", ev.Words, ev.Engine, ev.OutputPath)
		case pipeline.ErrEvent:
			fmt.Println("×", string(ev))
		}
	}
}
