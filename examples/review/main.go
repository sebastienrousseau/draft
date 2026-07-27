// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Command review runs draft's enhancement flow end to end against an
// in-process demo engine — no network, no LLM, no session CLI. It shows how
// --review works under the hood: the model sees only the article body (never
// the YAML frontmatter), returns exact find/replace edits grounded in the
// sources, and the runner applies them, re-attaches the frontmatter, and
// resyncs the body/yaml/final set.
//
// Run it with:
//
//	go run ./examples/review
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sebastienrousseau/draft/internal/config"
	"github.com/sebastienrousseau/draft/internal/engine"
	"github.com/sebastienrousseau/draft/internal/frontmatter"
	"github.com/sebastienrousseau/draft/internal/pipeline"
)

// demoEngine is a deterministic engine.Engine: it returns a fixed claim for
// extraction and one surgical edit for the review call.
type demoEngine struct{}

func (demoEngine) Name() string { return "demo" }

func (demoEngine) Generate(_ context.Context, req engine.Request) (engine.Result, error) {
	switch req.Kind {
	case engine.KindExtract:
		return engine.Result{Text: "CLAIM: Router-S used 5x fewer FLOPs\nSOURCE_QUOTE: \"used 5x fewer FLOPs\"\nTYPE: result\nSTRENGTH: demonstrated\n---"}, nil
	case engine.KindEdit:
		if strings.Contains(req.Prompt, "permalink:") {
			return engine.Result{}, fmt.Errorf("frontmatter leaked into the review prompt")
		}
		return engine.Result{Text: `[{"find":"One number tells the story.","replace":"A single number carries the argument.","reason":"generic"}]`}, nil
	default:
		return engine.Result{}, fmt.Errorf("unexpected request kind")
	}
}

func draftBody() string {
	prose := strings.Repeat("The grounded result stands on its own and reads plainly. ", 110)
	return "# Router-S Cuts Compute\n\n**One number tells the story.**\n\n" +
		"<aside class=\"post-lead\"><p><strong>TL;DR.</strong> Fewer FLOPs.</p></aside>\n\n" +
		"> **Executive Summary**\n>\n> - Router-S used 5x fewer FLOPs.\n\n## What it shows\n\n" + prose + "."
}

func main() {
	dir, err := os.MkdirTemp("", "draft-review-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	// A published article: frontmatter + body, saved as a -final.md file.
	body := draftBody()
	date := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	fm := frontmatter.Generate(body, date)
	draft := filepath.Join(dir, "2026-01-15-router-s-final.md")
	if err := os.WriteFile(draft, []byte(frontmatter.Combine(fm, body)), 0o644); err != nil {
		panic(err)
	}

	src := filepath.Join(dir, "router-s.txt")
	if err := os.WriteFile(src, []byte("Router-S used 5x fewer FLOPs than the dense baseline."), 0o644); err != nil {
		panic(err)
	}

	cfg := config.Config{HomeDir: dir, DraftsDir: dir, MaxContinue: 3}
	events := make(chan pipeline.Event, 256)
	go func() {
		pipeline.NewRunner(cfg, []engine.Engine{demoEngine{}}, events).
			Run(context.Background(), pipeline.Job{Sources: []string{src}, ReviewPath: draft})
		close(events)
	}()

	for e := range events {
		switch ev := e.(type) {
		case pipeline.LogEvent:
			fmt.Println("·", string(ev))
		case pipeline.DoneEvent:
			fmt.Printf("\n✓ enhanced in place (%d body words via %s)\n", ev.Words, ev.Engine)
		case pipeline.ErrEvent:
			fmt.Println("×", string(ev))
		}
	}

	// The edit landed, the frontmatter survived, and the set was resynced.
	enhanced, err := os.ReadFile(draft)
	if err != nil {
		panic(err)
	}
	fmt.Println("\nedit applied:          ", strings.Contains(string(enhanced), "A single number carries the argument."))
	fmt.Println("frontmatter intact:    ", strings.HasPrefix(string(enhanced), "---"))
	_, bodyErr := os.Stat(filepath.Join(dir, "2026-01-15-router-s-body.md"))
	_, yamlErr := os.Stat(filepath.Join(dir, "2026-01-15-router-s-frontmatter.yaml"))
	fmt.Println("siblings resynced:     ", bodyErr == nil && yamlErr == nil)
}
