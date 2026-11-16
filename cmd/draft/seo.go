// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/sebastienrousseau/draft/internal/brand"
	"github.com/sebastienrousseau/draft/seo"
)

// maxSEOFile bounds what --seo reads: a final article is tens of kilobytes,
// so anything near this is not one.
const maxSEOFile = 8 << 20

// seoReport is the --json form of one article's check.
type seoReport struct {
	Path     string        `json:"path"`
	Findings []seo.Finding `json:"findings"`
}

// runSEO checks final articles against Google Search's published guidance.
// The files are checked as one set, so a title or description two of them
// share is reported on both. It exits 1 when any article has an error.
func runSEO(o options, stdout, stderr io.Writer) int {
	paths := append([]string{o.seoPath}, o.args...)
	articles := make([]seo.Article, 0, len(paths))
	for _, p := range paths {
		b, err := readSmall(p)
		if err != nil {
			fmt.Fprintln(stderr, "draft:", err)
			return 1
		}
		articles = append(articles, seo.Parse(p, string(b)))
	}
	results := seo.CheckSet(articles)
	failed := false
	for i, fs := range results {
		failed = failed || seo.HasErrors(fs)
		if o.jsonOut {
			printSEOJSON(stdout, articles[i].Path, fs)
		} else {
			printSEO(stdout, articles[i].Path, fs)
		}
	}
	if failed {
		return 1
	}
	return 0
}

func readSmall(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxSEOFile {
		return nil, fmt.Errorf("%s is %d bytes; the limit is %d", path, info.Size(), maxSEOFile)
	}
	return os.ReadFile(path)
}

func printSEOJSON(w io.Writer, path string, fs []seo.Finding) {
	if fs == nil {
		fs = []seo.Finding{}
	}
	b, _ := json.Marshal(seoReport{Path: path, Findings: fs}) // a struct of strings always marshals
	fmt.Fprintln(w, string(b))
}

func printSEO(w io.Writer, path string, fs []seo.Finding) {
	fmt.Fprintln(w, path)
	if len(fs) == 0 {
		fmt.Fprintln(w, "  "+brand.Help.Render("Follows Google Search guidance."))
		return
	}
	for _, f := range fs {
		label := brand.Help.Render(string(f.Severity))
		if f.Severity == seo.Error {
			label = brand.Accent.Render(string(f.Severity))
		}
		fmt.Fprintf(w, "  %-8s %s: %s\n           %s\n", label, f.Rule, f.Message, f.Source)
	}
}
