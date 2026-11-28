// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

// helpSections are the prose sections of --help after the flags, one entry
// per printed line. The ENVIRONMENT list is checked against every variable
// config reads by TestEveryConfigVariableIsDocumented.
var helpSections = []struct {
	title string
	lines []string
}{
	{"ENVIRONMENT", []string{
		"DRAFT_ENGINE, DRAFT_EXTRACT_ENGINE, DRAFT_WRITE_ENGINE, DRAFT_EDIT_ENGINE,",
		"DRAFT_READER, DRAFT_STYLE, DRAFT_MODEL_SESSION, DRAFT_MODEL, DRAFT_WRITE_MODEL,",
		"DRAFT_EXTRACT_MODEL, DRAFT_EDIT_MODEL, DRAFT_NUM_CTX, DRAFT_NUM_PREDICT,",
		"DRAFT_WRITE_RETRIES, DRAFT_MAX_CONTINUE, DRAFT_EXTRACT_CONCURRENCY,",
		"DRAFT_CALL_TIMEOUT, DRAFT_EXPERIMENTAL, DRAFT_STRICT_NUMBERS, DRAFT_SECOND_GATE,",
		"DRAFT_STRUCTURED_OUTPUT,",
		"DRAFT_DRAFTS_DIR, DRAFT_SOURCES_DIR, DRAFT_CACHE_DIR, DRAFT_NO_CACHE,",
		"DRAFT_CONFIG, DRAFT_NO_CONFIG, DRAFT_C2PA_CERT, DRAFT_C2PA_KEY, DRAFT_C2PA_ALG,",
		"DRAFT_SITE_* (publisher identity), DRAFT_SHOW_LOGO=0, OLLAMA_HOST",
	}},
	{"CONFIG", []string{
		"~/.config/draft/config.toml (user), or a draft.toml (project) once trusted with",
		"--trust-config, sets defaults. Environment variables and flags override it.",
	}},
	{"OUTPUT", []string{
		"Each draft is saved as a set under ~/Drop/Drafts/YYYY-MM-DD/:",
		"source/<stem>-body.md, yaml/<stem>-frontmatter.yaml, final/<stem>-final.md.",
		"Scratch files are removed unless --keep-artifacts.",
	}},
}

// helpLines returns the lines of the help section titled title, so the
// manpage prints the same list --help does rather than a copy of it.
func helpLines(title string) []string {
	for _, s := range helpSections {
		if s.title == title {
			return s.lines
		}
	}
	return nil
}
