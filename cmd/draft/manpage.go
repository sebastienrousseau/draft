// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/sebastienrousseau/draft/engine"
)

// writeManPage emits a roff manual page built from the same tables the help
// text renders.
//
// A committed .1 file is a copy of the CLI that nothing keeps honest: flags get
// added, the manpage does not, and the packaged documentation quietly describes
// a different program. Generating it from flagHelp and usageExamples means the
// two cannot disagree, and `make -f GNUmakefile install` produces it at build
// time rather than shipping a stale artefact.
func writeManPage(w io.Writer, version string, now time.Time) error {
	b := &strings.Builder{}

	fmt.Fprintf(b, ".TH DRAFT 1 %q %q \"User Commands\"\n",
		now.Format("2006-01-02"), "draft "+version)

	section(b, "NAME", "draft \\- turn research papers into grounded Markdown drafts")

	section(b, "SYNOPSIS",
		".B draft\n"+
			"[\\fIOPTIONS\\fR]\n"+
			"\\fISOURCE\\fR [\\fISOURCE\\fR...]")

	section(b, "DESCRIPTION",
		roff("draft turns research PDFs into publication-ready Markdown in which every "+
			"sentence is grounded in a fact it can prove.")+"\n.PP\n"+
			roff("Source text is split into sections and mined for claims. A claim survives "+
				"only if its supporting quote occurs verbatim in the section it came from, "+
				"and only surviving claims are given to the model that writes the article. "+
				"Verification therefore happens before writing, not after: a model that "+
				"never sees an unverified fact cannot repeat one.")+"\n.PP\n"+
			roff("Online, draft writes using the first installed AI coding-agent CLI through "+
				"that tool's own logged-in session, so no API token is ever read or stored. "+
				"Offline, it falls back to a local Ollama model.")+"\n.PP\n"+
			roff("Bare filenames resolve against the sources directory. Each source becomes "+
				"its own draft, processed as a queue."))

	// OPTIONS, from the same table --help renders.
	fmt.Fprintf(b, ".SH OPTIONS\n")
	for _, f := range flagHelp {
		fmt.Fprintf(b, ".TP\n.B %s\n%s\n", roff(f[0]), roff(f[1]))
	}

	section(b, "ENGINE",
		roff("In auto mode (the default) draft uses the first installed agent CLI, in this "+
			"preference order:")+"\n.PP\n"+
			roff(strings.Join(engine.ProviderNames(), ", "))+"\n.PP\n"+
			roff("Providers marked experimental have a correct invocation but unverified "+
				"article output, and auto mode skips them unless --experimental is given. "+
				"Any provider can be forced by name. If a session call fails because the "+
				"machine is offline, draft fails over to Ollama and stays there for the "+
				"rest of the run.")+"\n.PP\n"+
			roff("No provider is invoked with a flag that grants it tools: draft only ever "+
				"asks for text, and its prompts carry verbatim text from third-party "+
				"documents."))

	fmt.Fprintf(b, ".SH EXAMPLES\n")
	for _, ex := range usageExamples {
		fmt.Fprintf(b, ".TP\n.B %s\n%s\n", roff(ex[0]), roff(capitalise(ex[1])))
	}

	section(b, "ENVIRONMENT",
		roff("Flags beat environment variables, which beat defaults. Every numeric "+
			"variable is clamped at both ends; an out-of-range value is reported on "+
			"stderr rather than silently ignored.")+"\n.PP\n"+
			roff("DRAFT_ENGINE, DRAFT_EXTRACT_ENGINE, DRAFT_WRITE_ENGINE, DRAFT_EDIT_ENGINE, "+
				"DRAFT_MODEL_SESSION, DRAFT_MODEL, DRAFT_WRITE_MODEL, DRAFT_EXTRACT_MODEL, "+
				"DRAFT_EDIT_MODEL, DRAFT_NUM_CTX, DRAFT_NUM_PREDICT, DRAFT_WRITE_RETRIES, "+
				"DRAFT_MAX_CONTINUE, DRAFT_EXTRACT_CONCURRENCY, DRAFT_CALL_TIMEOUT, "+
				"DRAFT_EXPERIMENTAL, DRAFT_STRICT_NUMBERS, DRAFT_DRAFTS_DIR, "+
				"DRAFT_SOURCES_DIR, DRAFT_CACHE_DIR, DRAFT_NO_CACHE, DRAFT_SHOW_LOGO, "+
				"DRAFT_SITE_*, OLLAMA_HOST."))

	section(b, "FILES",
		".TP\n.I ~/Drop/Drafts/YYYY-MM-DD/\n"+
			roff("Output for the day, as a set: source/<stem>-body.md, "+
				"yaml/<stem>-frontmatter.yaml and final/<stem>-final.md. Override with --out.")+
			"\n.TP\n.I ~/Drop/Drafts/Sources/\n"+
			roff("Where bare filenames resolve from. Override with --sources-dir.")+
			"\n.TP\n.I $XDG_CACHE_HOME/draft/extract/\n"+
			roff("Cached claim extractions, addressed by content. Override with "+
				"DRAFT_CACHE_DIR; empty with --clear-cache."))

	section(b, "EXIT STATUS",
		".TP\n.B 0\n"+roff("Every job produced a draft.")+
			"\n.TP\n.B 1\n"+roff("At least one job failed.")+
			"\n.TP\n.B 2\n"+roff("The command line or configuration was rejected."))

	section(b, "REQUIREMENTS",
		roff("pdftotext (Poppler) for PDF sources and textutil for DOCX on macOS, plus "+
			"either an installed agent CLI or a running Ollama server. Run "+
			"draft --doctor to see what is present."))

	section(b, "SEE ALSO",
		roff("Project documentation: https://draftlib.com")+"\n.PP\n"+
			roff("Source and issue tracker: https://github.com/sebastienrousseau/draft"))

	_, err := io.WriteString(w, b.String())
	return err
}

func section(b *strings.Builder, name, body string) {
	fmt.Fprintf(b, ".SH %s\n%s\n", name, body)
}

// roff escapes the characters that would otherwise be read as formatting. A
// leading dot starts a request, and a backslash starts an escape; a hyphen
// must be escaped so it renders as a hyphen rather than a soft break.
func roff(s string) string {
	s = strings.ReplaceAll(s, `\`, `\e`)
	s = strings.ReplaceAll(s, "-", `\-`)
	if strings.HasPrefix(s, ".") || strings.HasPrefix(s, "'") {
		s = `\&` + s
	}
	return s
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
