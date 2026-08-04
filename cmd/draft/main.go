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
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/frontmatter"
	"github.com/sebastienrousseau/draft/internal/brand"
	"github.com/sebastienrousseau/draft/internal/tui"
	"github.com/sebastienrousseau/draft/pipeline"
)

// version is the build version. Release builds inject it via -ldflags
// "-X main.version=…" (see .goreleaser.yaml); otherwise it is read from the
// module's build info, so a `go install` reports the tag it came from. There
// is deliberately no hand-maintained literal here — a second source of truth
// is how v0.0.22 once shipped mislabelled.
var version = buildVersion()

func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return "dev"
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// run is the testable core of the command: it parses argv, plans jobs, and
// dispatches to the headless runner or the TUI, returning a process exit code.
func run(argv []string, stdout, stderr io.Writer) int {
	flags := config.Flags{}
	var showVersion, headless bool
	var jsonOut, dryRun bool
	var reviewPath, frontmatterPath, completionShell string

	fs := flag.NewFlagSet("draft", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { usage(stderr) }
	fs.StringVar(&flags.Engine, "engine", "", "backend: auto (default), ollama, or a provider name")
	fs.StringVar(&flags.ExtractEngine, "extract-engine", "", "backend for claim extraction (default: --engine)")
	fs.StringVar(&flags.WriteEngine, "write-engine", "", "backend for writing the article (default: --engine)")
	fs.StringVar(&flags.Model, "model", "", "session-provider model override (e.g. opus)")
	fs.StringVar(&flags.Model, "claude-model", "", "deprecated alias for --model")
	fs.IntVar(&flags.ContextLength, "num-ctx", 0, "Ollama context window (default 8192)")
	fs.IntVar(&flags.PredictLength, "num-predict", 0, "Ollama max output tokens (default 6000)")
	fs.BoolVar(&flags.ForceNew, "force-new", false, "draft even if today's folder already has one")
	fs.BoolVar(&flags.Merge, "merge", false, "combine all sources into one draft instead of queueing")
	fs.BoolVar(&flags.Resume, "resume", false, "reuse a verified claim ledger from an earlier attempt")
	fs.BoolVar(&flags.KeepArtifacts, "keep-artifacts", false, "keep prompt/ledger files beside a successful draft")
	fs.BoolVar(&flags.Experimental, "experimental", false, "let auto mode use experimental (unverified) providers")
	fs.StringVar(&reviewPath, "review", "", "enhance an existing draft with surgical edits grounded in the sources")
	fs.StringVar(&frontmatterPath, "frontmatter", "", "generate/regenerate frontmatter and combined final article from a Markdown draft")
	fs.StringVar(&frontmatterPath, "combine", "", "alias for --frontmatter")
	fs.BoolVar(&headless, "print", false, "run without the TUI; print draft paths to stdout")
	fs.BoolVar(&jsonOut, "json", false, "run without the TUI; print one JSON object per job to stdout")
	fs.StringVar(&completionShell, "completion", "", "print a shell completion script: bash, zsh, or fish")
	fs.BoolVar(&dryRun, "dry-run", false, "report what a run would do, without calling a model")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")

	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if showVersion {
		fmt.Fprintln(stdout, "draft "+version)
		return 0
	}
	if completionShell != "" {
		if !writeCompletion(stdout, completionShell, engine.ProviderNames()) {
			fmt.Fprintf(stderr, "draft: unknown shell %q (want bash, zsh, or fish)\n", completionShell)
			return 2
		}
		return 0
	}

	if frontmatterPath != "" {
		if reviewPath != "" {
			fmt.Fprintln(stderr, "draft: --frontmatter cannot be combined with --review")
			return 2
		}
		if len(fs.Args()) > 0 {
			fmt.Fprintln(stderr, "draft: --frontmatter takes no source arguments")
			return 2
		}
		bodyPath, fmPath, finalPath, err := frontmatter.ProcessFile(frontmatterPath, time.Now())
		if err != nil {
			fmt.Fprintln(stderr, "draft:", err)
			return 1
		}
		fmt.Fprintln(stdout, "Frontmatter regenerated successfully:")
		fmt.Fprintln(stdout, "  Body file:       ", bodyPath)
		fmt.Fprintln(stdout, "  Frontmatter file:", fmPath)
		fmt.Fprintln(stdout, "  Final article:   ", finalPath)
		return 0
	}

	cfg := config.Load(flags)
	// Resolution problems that were recovered from — an unreadable home
	// directory, an out-of-range tunable, a non-loopback Ollama host — are
	// reported rather than applied in silence.
	for _, w := range cfg.Warnings {
		fmt.Fprintln(stderr, "draft: warning:", w)
	}
	// A misspelled provider name would otherwise degrade to Ollama without a
	// word, producing a local-model draft the user believes came from Claude.
	if err := engine.Validate(cfg); err != nil {
		fmt.Fprintln(stderr, "draft:", err)
		return 2
	}

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
	// One Runner for the whole queue. Constructing one per job reset the
	// fallback cursor every time, so a dead provider was retried — and
	// re-reported — once per paper instead of once per run.
	runner := pipeline.NewRoutedRunner(cfg, nil)

	if dryRun {
		if runDryRun(ctx, cfg, runner, jobs, stdout, stderr) > 0 {
			return 1
		}
		return 0
	}
	if jsonOut {
		if runHeadlessJSON(ctx, cfg, runner, jobs, stdout, stderr) > 0 {
			return 1
		}
		return 0
	}
	if headless {
		if runHeadless(ctx, cfg, runner, jobs, stdout, stderr) > 0 {
			return 1
		}
		return 0
	}

	selectEngine := (flags.Engine == "")
	if err := runTUI(ctx, stop, cfg, runner, jobs, selectEngine); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// runTUI launches the full-screen dashboard. It is separated from main so the
// job-planning logic around it stays testable. cancel is invoked when the user
// quits so background pipeline work stops promptly.
func runTUI(ctx context.Context, cancel context.CancelFunc, cfg config.Config, runner *pipeline.Runner, jobs []pipeline.Job, selectEngine bool) error {
	defer cancel()
	tui.Version = version
	m := tui.New(ctx, cancel, cfg, runner, jobs, selectEngine)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen(), tea.WithMouseCellMotion())
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

// usage prints the branded help: the logo and wordmark, then coral section
// headings over the reference text. Colour is dropped automatically when the
// output is not a terminal, and DRAFT_SHOW_LOGO=0 suppresses the logo.
func usage(w io.Writer) {
	head := func(s string) string { return brand.Title.Render(s) }
	flag := func(s string) string { return brand.Accent.Render(s) }
	dim := func(s string) string { return brand.Help.Render(s) }

	if brand.ShowLogo() {
		fmt.Fprint(w, brand.Logo(false))
	} else {
		fmt.Fprint(w, "  "+brand.Accent.Render(brand.Wordmark)+"  "+brand.Subtle.Render(brand.Tagline)+"\n\n")
	}
	fmt.Fprintf(w, "  %s\n\n", dim("draft "+version+" — research PDFs into grounded Markdown drafts"))

	fmt.Fprintf(w, "%s\n  draft [flags] <source> [more-sources...]\n\n", head("USAGE"))
	fmt.Fprintf(w, "  %s\n  %s\n\n",
		dim("Bare filenames resolve against ~/Drop/Drafts/Sources."),
		dim("Each source becomes its own draft, processed as a queue."))

	fmt.Fprintf(w, "%s\n", head("EXAMPLES"))
	for _, ex := range [][2]string{
		{`draft "2603.23420.pdf"`, "one paper"},
		{"draft a.pdf b.pdf c.pdf", "three drafts, queued"},
		{"draft --merge notes.md paper.pdf", "combine into a single draft"},
		{"draft --engine ollama paper.pdf", "force the local model"},
		{"draft --review draft.md paper.pdf", "enhance an existing draft"},
		{"draft --frontmatter source/x-body.md", "regenerate the yaml + final set"},
	} {
		fmt.Fprintf(w, "  %-40s %s\n", ex[0], dim("# "+ex[1]))
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "%s\n", head("ENGINE"))
	fmt.Fprintf(w, `  In auto mode (default) draft writes with the first installed AI coding-agent
  CLI, using that tool's own logged-in session — no API token. Supported
  providers, in preference order:

    %s

  claude, copilot, codex, grok, agy, and cursor-agent are verified end to end and
  used by auto mode; the rest are experimental (invocation correct, output
  unverified) and used by auto only with --experimental. Force any by name.

  If a session call fails because the machine is offline, draft fails over to a
  local Ollama model and stays there for the rest of the run.

`, strings.Join(engine.ProviderNames(), ", "))

	fmt.Fprintf(w, "%s\n", head("FLAGS"))
	for _, f := range [][2]string{
		{"--engine <mode>", "auto (default), ollama, or a provider name"},
		{"--extract-engine <m>", "backend for claim extraction (default: --engine)"},
		{"--write-engine <m>", "backend for writing (default: --engine)"},
		{"--model <name>", "session-provider model override (e.g. opus)"},
		{"--experimental", "let auto mode use experimental providers"},
		{"--num-ctx <n>", "Ollama context window (default 8192)"},
		{"--num-predict <n>", "Ollama max output tokens (default 6000)"},
		{"--force-new", "draft even if today's folder already has one"},
		{"--merge", "combine all sources into one draft"},
		{"--resume", "reuse a verified claim ledger from an earlier attempt"},
		{"--review <draft.md>", "enhance an existing draft with surgical edits"},
		{"--frontmatter <file>", "regenerate frontmatter and the final article"},
		{"--combine <file>", "alias for --frontmatter"},
		{"--keep-artifacts", "keep prompt/ledger files beside a successful draft"},
		{"--print", "run without the TUI; print draft paths to stdout"},
		{"--json", "run without the TUI; one JSON object per job on stdout"},
		{"--dry-run", "report what a run would do, without calling a model"},
		{"--completion <sh>", "print a completion script: bash, zsh, or fish"},
		{"--version", "print version and exit"},
		{"-h, --help", "show this help"},
	} {
		fmt.Fprintf(w, "  %s%s%s\n", flag(f[0]), strings.Repeat(" ", max(1, 23-len(f[0]))), dim(f[1]))
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "%s\n", head("ENVIRONMENT"))
	fmt.Fprintf(w, "  %s\n  %s\n  %s\n  %s\n\n",
		dim("DRAFT_ENGINE, DRAFT_MODEL_SESSION, DRAFT_MODEL, DRAFT_WRITE_MODEL,"),
		dim("DRAFT_EXTRACT_MODEL, DRAFT_EDIT_MODEL, DRAFT_NUM_CTX, DRAFT_NUM_PREDICT,"),
		dim("DRAFT_WRITE_RETRIES, DRAFT_MAX_CONTINUE, DRAFT_EXTRACT_CONCURRENCY,"),
		dim("DRAFT_SITE_* (publisher identity), DRAFT_SHOW_LOGO=0, OLLAMA_HOST"))

	fmt.Fprintf(w, "%s\n", head("OUTPUT"))
	fmt.Fprintf(w, "  %s\n  %s\n  %s\n\n",
		dim("Each draft is saved as a set under ~/Drop/Drafts/YYYY-MM-DD/:"),
		dim("source/<stem>-body.md, yaml/<stem>-frontmatter.yaml, final/<stem>-final.md."),
		dim("Scratch files are removed unless --keep-artifacts."))

	fmt.Fprintf(w, "%s\n  %s\n\n", head("REQUIREMENTS"),
		dim("pdftotext (Poppler) for PDFs, textutil for DOCX, plus either a session CLI (online) or a running Ollama server (offline)."))

	fmt.Fprintf(w, "%s\n  %s\n", head("KEYS"),
		dim("q / esc quit · enter queue another source · j/k · arrows · pgup/pgdn scroll"))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
