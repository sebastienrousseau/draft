// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package frontmatter_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/sebastienrousseau/draft/frontmatter"
)

// ExtractMetadata reads the article body: the H1 is the title, a deck
// paragraph or bold line is the subtitle, and the category is scored from
// the text rather than taken from the first keyword that matches.
func ExampleExtractMetadata() {
	meta := frontmatter.ExtractMetadata("# Agent Autonomy in Banking\n\n" +
		"<p class=\"deck\">What boards must certify before agents act.</p>\n")
	fmt.Println(meta.Title)
	fmt.Println(meta.Subtitle)
	fmt.Println(meta.Slug)
	// Output:
	// Agent Autonomy in Banking
	// What boards must certify before agents act.
	// agent-autonomy-in-banking
}

// Split separates an existing frontmatter block from the body, and Combine
// puts them back together — the round trip is lossless.
func ExampleSplit() {
	doc := "---\ntitle: \"Kept\"\n---\n\n# Body\n\nProse."
	fm, body := frontmatter.Split(doc)
	fmt.Println(len(fm) > 0, body)
	// Output: true # Body
	//
	// Prose.
}

// GenerateWithOptions stamps your own publisher identity, and any field
// already present in Existing wins over a generated one.
func ExampleGenerateWithOptions() {
	site := frontmatter.Site{BaseURL: "https://example.org", Name: "Ada"}
	yaml := frontmatter.GenerateWithOptions("# Title\n\nBody.", frontmatter.Options{
		Date:     time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Slug:     "my-canonical-slug",
		Site:     &site,
		Existing: map[string]string{"category": `"Technology"`},
	})
	for _, line := range []string{`url: "https://example.org/2026-01-15-my-canonical-slug"`, `category: "Technology"`} {
		fmt.Println(contains(yaml, line))
	}
	// Output:
	// true
	// true
}

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }
