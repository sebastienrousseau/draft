# draft/frontmatter

Metadata extraction, YAML frontmatter generation, and identity-preserving
regeneration of an article set.

[![Go reference](https://img.shields.io/badge/go.dev-reference-00ADD8?style=flat-square&logo=go&logoColor=white)](https://pkg.go.dev/github.com/sebastienrousseau/draft/frontmatter)

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [API](#api)
- [The three regeneration rules](#the-three-regeneration-rules)
- [Publisher identity](#publisher-identity)
- [License](#license)

## Install

```sh
go get github.com/sebastienrousseau/draft@latest
```

```go
import "github.com/sebastienrousseau/draft/frontmatter"
```

## Quick start

```go
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sebastienrousseau/draft/frontmatter"
)

func main() {
	body := "# Router-S Cuts Compute\n\n**One number tells the story.**\n\nRouter-S used 5x fewer FLOPs.\n"

	meta := frontmatter.ExtractMetadata(body) // title, subtitle, keywords, category
	fmt.Println("title:", meta.Title)

	site := frontmatter.DefaultSite
	site.Name = "My Site"

	yaml := frontmatter.GenerateWithOptions(body, frontmatter.Options{
		Date:     time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC),
		Slug:     "router-s-cuts-compute",                     // the filename is the identity, not the headline
		Site:     &site,                                       // nil selects frontmatter.DefaultSite
		Existing: map[string]string{"author": "Ada Lovelace"}, // curated fields always win
	})
	doc := frontmatter.Combine(yaml, body)
	fmt.Printf("combined document: %d bytes\n", len(doc))

	// Split is Combine's inverse, up to surrounding whitespace: both trim, so
	// the round trip is stable rather than byte-identical to a padded input.
	gotYAML, gotBody := frontmatter.Split(doc)
	fmt.Println(len(gotYAML) > 0, gotBody == strings.TrimSpace(body))

	// ProcessFile writes the body/yaml/final set beside the input.
	dir, err := os.MkdirTemp("", "draft-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "2026-07-29-router-s-cuts-compute-body.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		log.Fatal(err)
	}

	bodyPath, yamlPath, finalPath, err := frontmatter.ProcessFile(path, time.Now())
	if err != nil {
		log.Fatalf("regenerating the article set: %v", err)
	}
	fmt.Println(filepath.Base(bodyPath), filepath.Base(yamlPath), filepath.Base(finalPath))
}
```

## API

| Symbol | Signature | Purpose |
| ------ | --------- | ------- |
| `ExtractMetadata` | `func(markdown string) Metadata` | Title, subtitle, description, excerpt, keywords, tags, category, slug |
| `Generate` | `func(markdown string, date time.Time) string` | Frontmatter from the body alone |
| `GenerateWithOptions` | `func(markdown string, opts Options) string` | Same, honouring slug, site and existing fields |
| `Combine` | `func(frontmatterYAML, body string) string` | Join into one publishable document |
| `Split` | `func(content string) (string, string)` | Inverse of `Combine` |
| `ProcessFile` | `func(inputPath string, date time.Time) (bodyPath, frontmatterPath, finalPath string, err error)` | Write or rewrite the whole set |
| `PartOfSet` | `func(path string) bool` | Whether a path belongs to a generated set |
| `Slugify` | `func(s string) string` | URL-friendly slug |
| `Site` / `DefaultSite` / `SiteFromEnv` | publisher identity | See below |
| `Options` | `struct{ Date time.Time; Slug string; Existing map[string]string; Site *Site }` | Generation inputs beyond the body |

## The three regeneration rules

`ProcessFile` is safe to run at any time because of three guarantees:

1. **The filename is the article's identity.** A `YYYY-MM-DD` prefix and the
   slug in the filename drive every URL in the frontmatter. Retitle the article
   and the permalink holds.
2. **Your edits always win.** Values in `Options.Existing` — read back from the
   prior frontmatter — are preserved verbatim; only missing fields are
   generated. Delete a field to have it rebuilt.
3. **Unchanged input is a no-op.** Reprocessing a set that has not changed
   rewrites every file byte for byte identically.

The set is three files, written side by side:

```text
2026-07-29/
├── source/2026-07-29-<slug>-body.md         # the article — edit this
├── yaml/2026-07-29-<slug>-frontmatter.yaml  # adjacent frontmatter
└── final/2026-07-29-<slug>-final.md         # combined, ready to publish
```

## Publisher identity

`Site` holds the identity stamped into generated frontmatter: base URL, CDN,
display name, short name, email, Twitter handle, location, measurement ID and
the first year of the copyright range.

`Options.Site` of `nil` selects `DefaultSite`. `SiteFromEnv` returns
`DefaultSite` with any `DRAFT_SITE_*` environment overrides applied, which is
what the CLI uses. Curated frontmatter fields still win over anything generated
from a `Site`.

## License

Licensed under either of [Apache License 2.0](../LICENSE-APACHE) or
[MIT License](../LICENSE-MIT), at your option. © Sebastien Rousseau.

<p align="right"><a href="#draftfrontmatter">Back to top ↑</a></p>
