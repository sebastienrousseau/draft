// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Command draft turns research PDFs into grounded, body-only Markdown article
// drafts. When the machine is online it writes with the first installed AI
// coding-agent CLI (Claude, Codex, Gemini, Copilot, Cursor, Amp, Crush, Goose,
// Grok, Qwen, ...) using that tool's own logged-in session — no API token.
// Offline, it falls back to a local Ollama model. Pass one or more sources;
// each becomes its own draft, processed as a queue in a full-screen dashboard.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sebastienrousseau/draft/internal/config"
	"github.com/sebastienrousseau/draft/internal/engine"
	"github.com/sebastienrousseau/draft/internal/pipeline"
	"github.com/sebastienrousseau/draft/internal/tui"
)

// version is the build version, overridden at release time via -ldflags
// "-X main.version=…" (see .goreleaser.yaml).
var version = "0.0.4"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// run is the testable core of the command: it parses argv, plans jobs, and
// dispatches to the headless runner or the TUI, returning a process exit code.
func run(argv []string, stdout, stderr io.Writer) int {
	flags := config.Flags{}
	var showVersion, headless bool
	var reviewPath string

	fs := flag.NewFlagSet("draft", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { usage(stderr) }
	fs.StringVar(&flags.Engine, "engine", "", "backend: auto (default), ollama, or a provider name")
	fs.StringVar(&flags.Model, "model", "", "session-provider model override (e.g. opus)")
	fs.StringVar(&flags.Model, "claude-model", "", "deprecated alias for --model")
	fs.IntVar(&flags.ContextLength, "num-ctx", 0, "Ollama context window (default 8192)")
	fs.IntVar(&flags.PredictLength, "num-predict", 0, "Ollama max output tokens (default 6000)")
	fs.BoolVar(&flags.ForceNew, "force-new", false, "draft even if today's folder already has one")
	fs.BoolVar(&flags.Merge, "merge", false, "combine all sources into one draft instead of queueing")
	fs.BoolVar(&flags.KeepArtifacts, "keep-artifacts", false, "keep prompt/ledger files beside a successful draft")
	fs.BoolVar(&flags.Experimental, "experimental", false, "let auto mode use experimental (unverified) providers")
	fs.StringVar(&reviewPath, "review", "", "enhance an existing draft with surgical edits grounded in the sources")
	fs.BoolVar(&headless, "print", false, "run without the TUI; print draft paths to stdout")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")

	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if showVersion {
		fmt.Fprintln(stdout, "draft "+version)
		return 0
	}

	cfg := config.Load(flags)
	args := fs.Args()
	if len(args) == 0 {
		usage(stderr)
		return 2
	}

	jobs, err := buildJobs(cfg, args, reviewPath)
	if err != nil {
		fmt.Fprintln(stderr, "draft:", err)
		return 1
	}

	// A signal-aware context so Ctrl+C (headless) and quitting the TUI abort any
	// in-flight session subprocess or Ollama request instead of orphaning it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	engines := engine.Chain(cfg)

	if headless {
		if runHeadless(ctx, cfg, engines, jobs, stdout, stderr) > 0 {
			return 1
		}
		return 0
	}

	if err := runTUI(ctx, stop, cfg, engines, jobs); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// runTUI launches the full-screen dashboard. It is separated from main so the
// job-planning logic around it stays testable. cancel is invoked when the user
// quits so background pipeline work stops promptly.
func runTUI(ctx context.Context, cancel context.CancelFunc, cfg config.Config, engines []engine.Engine, jobs []pipeline.Job) error {
	defer cancel()
	m := tui.New(ctx, cancel, cfg, engines, jobs)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

// buildJobs resolves each argument to an absolute source path and groups them
// into jobs: one per source by default, a single merged job with --merge, or a
// single review job when reviewPath is set (enhance that draft from the sources).
func buildJobs(cfg config.Config, args []string, reviewPath string) ([]pipeline.Job, error) {
	var sources []string
	for _, arg := range args {
		path, err := resolveSource(cfg, arg)
		if err != nil {
			return nil, err
		}
		sources = append(sources, path)
	}
	if reviewPath != "" {
		if len(sources) == 0 {
			return nil, fmt.Errorf("--review needs at least one source to ground the edits")
		}
		abs, err := filepath.Abs(reviewPath)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(abs); err != nil {
			return nil, fmt.Errorf("draft to review not found: %s", reviewPath)
		}
		return []pipeline.Job{{Sources: sources, ReviewPath: abs}}, nil
	}
	if cfg.Merge {
		return []pipeline.Job{{Sources: sources}}, nil
	}
	jobs := make([]pipeline.Job, 0, len(sources))
	for _, s := range sources {
		jobs = append(jobs, pipeline.Job{Sources: []string{s}})
	}
	return jobs, nil
}

// resolveSource expands ~, resolves bare filenames against the Sources
// directory, and confirms the file exists.
func resolveSource(cfg config.Config, arg string) (string, error) {
	expanded := arg
	if arg == "~" {
		expanded = cfg.HomeDir
	} else if strings.HasPrefix(arg, "~/") {
		expanded = filepath.Join(cfg.HomeDir, arg[2:])
	}
	candidates := []string{expanded}
	if !filepath.IsAbs(expanded) {
		candidates = append(candidates, filepath.Join(cfg.SourcesDir, expanded))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs, nil
		}
	}
	return "", fmt.Errorf("research file not found: %s", arg)
}

func usage(w io.Writer) {
	fmt.Fprint(w, `draft `+version+` — research PDFs into grounded Markdown drafts

USAGE
  draft [flags] <source> [more-sources...]

  Bare filenames resolve against ~/Drop/Drafts/Sources.
  Each source becomes its own draft, processed as a queue.

EXAMPLES
  draft "2603.23420.pdf"                 # one paper
  draft a.pdf b.pdf c.pdf                # three drafts, queued
  draft --merge notes.md paper.pdf       # combine into a single draft
  draft --engine ollama paper.pdf        # force the local model
  draft --engine codex paper.pdf         # force a specific session provider
  draft --model opus paper.pdf           # override the session model

ENGINE
  In auto mode (default) draft writes with the first installed AI coding-agent
  CLI, using that tool's own logged-in session — no API token. Supported
  providers, in preference order:

    `+strings.Join(engine.ProviderNames(), ", ")+`

  claude, copilot, codex, and grok are verified end to end and used by auto mode;
  the rest are experimental (invocation correct, output unverified) and are used
  by auto only with --experimental. Any provider can still be forced by name.

  If a session call fails because the machine is offline, draft fails over to a
  local Ollama model and stays there for the rest of the run. Force any backend
  with --engine.

FLAGS
  --engine <mode>        auto (default), ollama, or a provider name
  --model <name>         session-provider model override (e.g. opus)
  --experimental         let auto mode use experimental providers
  --num-ctx <n>          Ollama context window (default 8192)
  --num-predict <n>      Ollama max output tokens (default 6000)
  --force-new            draft even if today's folder already has one
  --merge                combine all sources into one draft
  --review <draft.md>    enhance an existing draft with surgical edits
  --keep-artifacts       keep prompt/ledger files beside a successful draft
  --print                run without the TUI; print draft paths to stdout
  --version              print version and exit
  -h, --help             show this help

ENVIRONMENT
  DRAFT_ENGINE, DRAFT_MODEL_SESSION, DRAFT_MODEL, DRAFT_WRITE_MODEL,
  DRAFT_EXTRACT_MODEL, DRAFT_EDIT_MODEL, DRAFT_NUM_CTX, DRAFT_NUM_PREDICT,
  DRAFT_WRITE_RETRIES, DRAFT_MAX_CONTINUE, OLLAMA_HOST

OUTPUT
  A successful run leaves only the finished article in ~/Drop/Drafts/YYYY-MM-DD/
  (prompt and ledger scratch files are removed unless --keep-artifacts). A
  failed run keeps the raw output and any needs-review copy for inspection.

REQUIREMENTS
  pdftotext (Poppler) for PDFs, textutil for DOCX, plus either a session CLI
  (online) or a running Ollama server (offline).

KEYS
  q / esc quit · enter queue another source · j/k · arrows · pgup/pgdn scroll
`)
}
