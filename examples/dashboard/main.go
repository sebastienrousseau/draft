// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Command dashboard runs draft's full-screen dashboard against an in-process
// demo engine — no model, no network, no session CLI, and no source document
// of your own. The article streams in word by word so the live preview, the
// phase markers, the progress bar, and the focus timer all animate exactly as
// they do on a real run.
//
// Use it to see the interface, and to check the layout at different terminal
// sizes: resize the window while it runs, or try it in a short one. The logo
// needs 24 rows; below that the header collapses to a one-line masthead, and
// DRAFT_SHOW_LOGO=0 turns it off entirely.
//
// Run it with:
//
//	go run ./examples/dashboard
//
// Press q to quit.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/tui"
	"github.com/sebastienrousseau/draft/pipeline"
)

// demoEngine is a deterministic engine.Engine that paces itself like a real
// backend: extraction pauses briefly per section, and the article is streamed
// a word at a time.
type demoEngine struct{}

func (demoEngine) Name() string { return "demo" }

func (demoEngine) Generate(ctx context.Context, req engine.Request) (engine.Result, error) {
	if req.Kind == engine.KindExtract {
		if err := pause(ctx, 900*time.Millisecond); err != nil {
			return engine.Result{}, err
		}
		return engine.Result{Text: "CLAIM: Router-S used 5x fewer FLOPs\n" +
			"SOURCE_QUOTE: \"used 5x fewer FLOPs\"\nTYPE: result\nSTRENGTH: demonstrated\n---"}, nil
	}

	var sb strings.Builder
	for _, word := range strings.Split(article(), " ") {
		if err := pause(ctx, 6*time.Millisecond); err != nil {
			return engine.Result{}, err
		}
		chunk := word + " "
		sb.WriteString(chunk)
		if req.OnChunk != nil {
			req.OnChunk(chunk)
		}
	}
	return engine.Result{Text: sb.String()}, nil
}

// pause sleeps unless the run is cancelled first, so quitting the dashboard
// stops the demo immediately.
func pause(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func article() string {
	body := strings.Repeat("The grounded result stands on its own and reads plainly. ", 110)
	return "# Router-S Cuts Compute Without Cutting Accuracy\n\n" +
		"**A single number carries the argument.**\n\n" +
		"<aside class=\"post-lead\"><p><strong>TL;DR.</strong> Router-S matches the dense " +
		"baseline with 5x fewer FLOPs.</p></aside>\n\n" +
		"> **Executive Summary**\n>\n> - Router-S used 5x fewer FLOPs.\n\n" +
		"## What the result shows\n\n" + body + "."
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "dashboard:", err)
		os.Exit(1)
	}
	fmt.Println("Demo finished. Drafts were written to a temporary directory and removed.")
}

func run() error {
	dir, err := os.MkdirTemp("", "draft-dashboard-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	// Two sources, so the queue has something to show.
	var sources []string
	for _, name := range []string{"router-s.txt", "follow-up-notes.txt"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("Router-S used 5x fewer FLOPs than the dense baseline."), 0o644); err != nil {
			return err
		}
		sources = append(sources, p)
	}

	jobs := []pipeline.Job{{Sources: sources[:1]}, {Sources: sources[1:]}}
	cfg := config.Config{HomeDir: dir, DraftsDir: dir, MaxContinue: 3}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tui.Version = "demo"
	m := tui.New(ctx, cancel, cfg, pipeline.NewRunner(cfg, []engine.Engine{demoEngine{}}, nil), jobs)
	_, err = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}
