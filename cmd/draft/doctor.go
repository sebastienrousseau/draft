// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/brand"
)

// probes are the environment lookups the doctor performs, injectable so the
// report can be tested without a particular machine's tooling installed.
type probes struct {
	lookPath   func(string) (string, error)
	ollamaUp   func(string) bool
	writable   func(string) error
	goos       string
	providerOK func(string) bool
}

func defaultProbes() probes {
	return probes{
		lookPath:   exec.LookPath,
		ollamaUp:   engine.IsOllamaRunning,
		writable:   dirWritable,
		goos:       runtime.GOOS,
		providerOK: engine.IsAvailable,
	}
}

// dirWritable reports whether a directory can be created and written to. The
// drafts directory is the one place a run must not discover a problem after
// spending ten minutes on a model.
func dirWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".draft-write-probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

// runDoctor reports whether this machine can run draft, and returns a process
// exit code: 0 when a draft could be produced now, 1 when something required
// is missing.
//
// Every requirement was previously discovered at failure time — after the user
// had committed to a run. --dry-run proves a source is readable; this proves
// the machine is ready.
func runDoctor(cfg config.Config, p probes, w io.Writer) int {
	head := func(s string) string { return brand.Title.Render(s) }
	dim := func(s string) string { return brand.Help.Render(s) }

	row := func(status, label, detail string) {
		fmt.Fprintf(w, "  %-3s %-22s %s\n", status, label, dim(detail))
	}
	ok, bad, note := "ok", "!!", "--"

	fmt.Fprintf(w, "%s\n", head("SOURCE TOOLING"))
	pdfOK := false
	if path, err := p.lookPath("pdftotext"); err == nil {
		pdfOK = true
		row(ok, "pdftotext", path)
	} else {
		row(bad, "pdftotext", "not on PATH — PDFs cannot be read (install Poppler)")
	}
	if p.goos == "darwin" {
		if path, err := p.lookPath("textutil"); err == nil {
			row(ok, "textutil", path)
		} else {
			row(note, "textutil", "not on PATH — DOCX sources unavailable")
		}
	} else {
		row(note, "textutil", "macOS only — DOCX sources unavailable on "+p.goos)
	}

	fmt.Fprintf(w, "\n%s\n", head("BACKENDS"))
	installed := 0
	for _, prov := range engine.Providers() {
		if !p.providerOK(prov.Name) {
			continue
		}
		installed++
		detail := "session provider"
		if prov.Experimental {
			detail += " (experimental; auto mode skips it without --experimental)"
		}
		row(ok, prov.Name, detail)
	}
	if installed == 0 {
		row(note, "session providers", "none installed — draft will use Ollama")
	}

	ollamaOK := false
	if _, err := p.lookPath("ollama"); err != nil {
		row(note, "ollama", "not on PATH — no offline fallback")
	} else if p.ollamaUp(cfg.OllamaHost) {
		ollamaOK = true
		row(ok, "ollama", "responding at "+cfg.OllamaHost)
	} else {
		row(note, "ollama", "installed but not responding at "+cfg.OllamaHost+" (draft will start it)")
		ollamaOK = true
	}

	fmt.Fprintf(w, "\n%s\n", head("PATHS"))
	pathsOK := true
	for _, d := range []struct{ label, dir string }{
		{"drafts (--out)", cfg.DraftsDir},
		{"sources", cfg.SourcesDir},
	} {
		if err := p.writable(d.dir); err != nil {
			pathsOK = false
			row(bad, d.label, d.dir+" — "+err.Error())
			continue
		}
		row(ok, d.label, d.dir)
	}
	if cfg.CacheDir == "" {
		row(note, "cache", "disabled; every run re-extracts")
	} else if err := p.writable(cfg.CacheDir); err != nil {
		row(note, "cache", cfg.CacheDir+" — "+err.Error()+" (runs will not be cached)")
	} else {
		row(ok, "cache", cfg.CacheDir)
	}

	fmt.Fprintf(w, "\n%s\n", head("ROUTING"))
	for _, k := range []struct {
		label string
		kind  engine.Kind
	}{
		{"extract", engine.KindExtract},
		{"write", engine.KindWrite},
		{"edit", engine.KindEdit},
	} {
		row(note, k.label, engine.NameFor(cfg, k.kind))
	}
	if cfg.StrictNumbers {
		row(note, "strict numbers", "on — an ungrounded number fails the draft")
	}

	fmt.Fprintln(w)
	switch {
	case !pdfOK && installed == 0 && !ollamaOK:
		fmt.Fprintln(w, "  "+brand.Accent.Render("Not ready.")+" No source tooling and no backend.")
		return 1
	case !pdfOK:
		fmt.Fprintln(w, "  "+brand.Accent.Render("Not ready.")+" Install Poppler for pdftotext, or supply Markdown or text sources.")
		return 1
	case installed == 0 && !ollamaOK:
		fmt.Fprintln(w, "  "+brand.Accent.Render("Not ready.")+" Install an agent CLI ("+strings.Join(engine.ProviderNames(), ", ")+") or Ollama.")
		return 1
	case !pathsOK:
		fmt.Fprintln(w, "  "+brand.Accent.Render("Not ready.")+" A required directory is not writable.")
		return 1
	}
	fmt.Fprintln(w, "  "+dim("Ready. Run `draft --dry-run <source>` to check a specific paper."))
	return 0
}
