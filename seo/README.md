# draft/seo

Checks a final article against Google Search guidance.

[![Go reference](https://img.shields.io/badge/go.dev-reference-00ADD8?style=flat-square&logo=go&logoColor=white)](https://pkg.go.dev/github.com/sebastienrousseau/draft/seo)
[![Website](https://img.shields.io/badge/draftlib.com-ff6b5a?style=flat-square)](https://draftlib.com)

Every rule enforces a sentence from Google Search Central and names the page
it comes from. Nothing Google says it ignores is checked: no length limits,
no word count, no keyword density, no meta keywords, no heading order.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [API](#api)
- [Rules](#rules)
- [License](#license)

## Install

```sh
go get github.com/sebastienrousseau/draft@latest
```

```go
import "github.com/sebastienrousseau/draft/seo"
```

## Quick start

```go
a := seo.Parse("post.md", finalArticle) // frontmatter + Markdown body
for _, f := range seo.Check(a) {
	fmt.Println(f.Severity, f.Rule, f.Message, f.Source)
}
```

## API

| Symbol      | Signature                              | Purpose                                                  |
| ----------- | -------------------------------------- | -------------------------------------------------------- |
| `Parse`     | `func(path, content string) Article`   | Read a final article's frontmatter fields and body       |
| `Check`     | `func(a Article) []Finding`            | Run every per-article rule                               |
| `CheckSet`  | `func(articles []Article) [][]Finding` | Per-article rules plus duplicate titles and descriptions |
| `HasErrors` | `func(fs []Finding) bool`              | Whether any finding is an `Error`                        |

## Rules

| Rule                          | Severity | Google page         |
| ----------------------------- | -------- | ------------------- |
| `title-missing`               | error    | Title links         |
| `title-boilerplate`           | error    | Title links         |
| `title-repeated-word`         | error    | Title links         |
| `title-duplicate`             | error    | Title links         |
| `description-missing`         | error    | Snippets            |
| `description-too-short`       | error    | Snippets            |
| `description-keyword-list`    | error    | Snippets            |
| `description-duplicate`       | error    | Snippets            |
| `description-truncated`       | warning  | Snippets            |
| `description-repeats-title`   | warning  | Snippets            |
| `image-alt-missing`           | error    | SEO starter guide   |
| `link-generic-text`           | warning  | SEO starter guide   |
| `link-to-site-root`           | warning  | Helpful content     |
| `heading-multiple-h1`         | warning  | Title links         |
| `byline-missing`              | error    | Helpful content     |
| `disclosure-missing`          | warning  | Helpful content     |
| `canonical-missing`           | error    | SEO starter guide   |
| `canonical-conflict`          | error    | SEO starter guide   |
| `structured-data-missing`     | warning  | SEO starter guide   |
| `structured-data-faq-retired` | warning  | FAQ structured data |

## License

Licensed under either of [Apache License 2.0](../LICENSE-APACHE) or
[MIT License](../LICENSE-MIT), at your option. © Sebastien Rousseau.

<p align="right"><a href="#draftseo">Back to top ↑</a></p>
