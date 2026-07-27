// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Command frontmatter demonstrates draft's article-set machinery with no LLM
// or network: metadata extraction from a Markdown body, YAML frontmatter
// generation with a custom publisher identity, the Split/Combine round trip,
// and ProcessFile's three-rule regeneration contract — the filename is the
// article's identity, existing frontmatter fields always win, and unchanged
// input is a byte-level no-op.
//
// Run it with:
//
//	go run ./examples/frontmatter
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sebastienrousseau/draft/internal/frontmatter"
)

const article = `# Router-S Cuts Compute Without Cutting Accuracy

<p class="deck">A sparse router matches the dense baseline with 5x fewer FLOPs. The interesting part is where the savings come from.</p>

<aside class="post-lead">
<p class="post-lead-tldr"><strong>TL;DR.</strong> Router-S reaches a validation loss of 3.41 with 5x fewer FLOPs than the dense baseline.</p>
</aside>

## What the result shows

The router learns to skip computation the dense model wastes on easy tokens.
`

func main() {
	// 1. Extract metadata from the body: title, deck subtitle, TL;DR
	// description, keywords, and a scored category.
	meta := frontmatter.ExtractMetadata(article)
	fmt.Println("── metadata extracted from the body ──")
	fmt.Printf("title:    %s\nsubtitle: %s\ncategory: %s\nslug:     %s\n\n",
		meta.Title, meta.Subtitle, meta.Category, meta.Slug)

	// 2. Generate frontmatter with your own publisher identity. Options.Site
	// replaces every hardcoded author/URL/handle; nil means DefaultSite.
	site := frontmatter.Site{
		BaseURL:       "https://blog.example.org",
		CDN:           "https://cdn.example.org",
		Name:          "Ada Example",
		ShortName:     "adaexample",
		Email:         "ada@example.org",
		TwitterHandle: "@ada",
		Location:      "Paris, FR",
		MeasurementID: "G-EXAMPLE1",
		CopyrightFrom: 2024,
	}
	date := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	fm := frontmatter.GenerateWithOptions(article, frontmatter.Options{Date: date, Site: &site})
	fmt.Println("── two generated fields (of ~70) ──")
	for _, line := range strings.Split(fm, "\n") {
		if strings.HasPrefix(line, "permalink:") || strings.HasPrefix(line, "description:") {
			fmt.Println(line)
		}
	}

	// 3. Combine and Split round-trip losslessly.
	if _, body := frontmatter.Split(frontmatter.Combine(fm, article)); strings.TrimSpace(body) != strings.TrimSpace(article) {
		panic("round trip lost the body")
	}
	fmt.Println("\nCombine → Split round trip: body intact")

	// 4. ProcessFile writes the three-file set. The filename's date and slug
	// are canonical — they drive every URL regardless of the headline.
	dir, err := os.MkdirTemp("", "draft-frontmatter-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	src := filepath.Join(dir, "2026-01-15-router-s-cuts-compute.md")
	if err := os.WriteFile(src, []byte(article), 0o644); err != nil {
		panic(err)
	}
	bodyPath, fmPath, finalPath, err := frontmatter.ProcessFile(src, time.Now())
	if err != nil {
		panic(err)
	}
	fmt.Println("\n── generated set ──")
	for _, p := range []string{bodyPath, fmPath, finalPath} {
		fmt.Println("  ", filepath.Base(p))
	}

	// 5. Existing fields always win: curate one field, regenerate, and the
	// curated value survives while everything else is untouched.
	curated, err := os.ReadFile(fmPath)
	if err != nil {
		panic(err)
	}
	edited := strings.Replace(string(curated),
		"description: ", `description: "A hand-written description that must survive." # was: `, 1)
	if err := os.WriteFile(fmPath, []byte(edited), 0o644); err != nil {
		panic(err)
	}
	if _, _, _, err := frontmatter.ProcessFile(bodyPath, time.Now()); err != nil {
		panic(err)
	}
	regenerated, err := os.ReadFile(fmPath)
	if err != nil {
		panic(err)
	}
	if !strings.Contains(string(regenerated), "A hand-written description that must survive.") {
		panic("curated field lost")
	}
	fmt.Println("\ncurated description survived regeneration")

	// 6. Unchanged input is a no-op: regenerate again, bytes identical.
	before, _ := os.ReadFile(finalPath)
	if _, _, _, err := frontmatter.ProcessFile(finalPath, time.Now()); err != nil {
		panic(err)
	}
	after, _ := os.ReadFile(finalPath)
	if string(before) != string(after) {
		panic("regeneration was not a no-op")
	}
	fmt.Println("reprocessing an unchanged set: byte-level no-op")
}
