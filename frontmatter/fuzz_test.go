// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package frontmatter

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

var slugCharset = regexp.MustCompile(`^[a-z0-9-]*$`)

// FuzzSplit drives the frontmatter/body separator with arbitrary documents.
// Splitting must never lose or invent body text, and must be stable: feeding a
// combined document back through Split has to yield the same body again.
func FuzzSplit(f *testing.F) {
	f.Add("---\ntitle: \"x\"\n---\n\n# Body\n\ntext")
	f.Add("# No frontmatter\n\njust prose")
	f.Add("---\n---\n")
	f.Add("---\nnote: value ending in ---")
	f.Add("---\r\ntitle: \"crlf\"\r\n---\r\n\r\n# Body")
	f.Add("")

	f.Fuzz(func(t *testing.T, doc string) {
		fm, body := Split(doc)
		if fm != "" && !strings.HasPrefix(fm, "---") {
			t.Errorf("frontmatter block does not open with a delimiter: %q", fm)
		}
		if !utf8.ValidString(doc) {
			return // invalid input may produce invalid output; only check well-formed docs
		}
		if !utf8.ValidString(fm) || !utf8.ValidString(body) {
			t.Errorf("Split produced invalid UTF-8 from valid input")
		}
		// Stability: recombining and re-splitting must reproduce the body.
		if body != "" {
			if _, again := Split(Combine(fm, body)); again != strings.TrimSpace(body) {
				t.Errorf("Split is not stable:\nfirst:  %q\nsecond: %q", body, again)
			}
		}
	})
}

// FuzzExtractMetadata drives metadata extraction with arbitrary Markdown. The
// output is written straight into YAML and URLs, so it must always be valid
// UTF-8, and the slug must stay inside the URL-safe charset at a bounded
// length however strange the heading is.
func FuzzExtractMetadata(f *testing.F) {
	f.Add("# Title\n\n<p class=\"deck\">Deck.</p>\n\nbody")
	f.Add("# " + strings.Repeat("é", 300) + "\n\nbody")
	f.Add("# C# and 100% > 50%\n\nbody")
	f.Add("---\nexisting: \"yes\"\n---\n\n# T\n\nbody")
	f.Add("")

	f.Fuzz(func(t *testing.T, markdown string) {
		if !utf8.ValidString(markdown) {
			return
		}
		m := ExtractMetadata(markdown)
		for name, field := range map[string]string{
			"title": m.Title, "subtitle": m.Subtitle, "description": m.Description,
			"excerpt": m.Excerpt, "keywords": m.Keywords, "tags": m.Tags,
			"category": m.Category, "slug": m.Slug,
		} {
			if !utf8.ValidString(field) {
				t.Errorf("%s is not valid UTF-8: %q", name, field)
			}
		}
		if !slugCharset.MatchString(m.Slug) {
			t.Errorf("slug leaves the URL-safe charset: %q", m.Slug)
		}
		if strings.HasPrefix(m.Slug, "-") || strings.HasSuffix(m.Slug, "-") {
			t.Errorf("slug has a dangling separator: %q", m.Slug)
		}
		if len(m.Slug) > 90 {
			t.Errorf("slug is %d bytes, over the 90 cap: %q", len(m.Slug), m.Slug)
		}
		if m.Category == "" {
			t.Error("category must always resolve to a value")
		}
	})
}
