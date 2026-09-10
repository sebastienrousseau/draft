// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package frontmatter extracts metadata from article Markdown and generates YAML
// frontmatter, as well as splitting and combining frontmatter and article bodies.
package frontmatter

import (
	"path/filepath"
	"regexp"
	"strings"
)

// MaxArticleBytes bounds how large a Markdown file ProcessFile will read. An
// article is prose; anything past this is a mistake, and reading it would cost
// several times its size once metadata extraction runs over it.
const MaxArticleBytes = 32 << 20

var (
	titlePat            = regexp.MustCompile(`(?m)^#\s+(.+)$`)
	boldLinePat         = regexp.MustCompile(`(?m)^\*\*(.+?)\*\*\s*$`)
	deckPat             = regexp.MustCompile(`(?is)<p\s+class="deck">(.*?)</p>`)
	tldrPat             = regexp.MustCompile(`(?i)<p\s+class="post-lead-tldr"><strong>TL;DR\.?</strong>\s*(.*?)</p>`)
	execSummaryPat      = regexp.MustCompile(`(?s)>\s*\*\*Executive Summary\*\*\s*\n(.*?)(?:\n\n|\n#|$)`)
	htmlTagPat          = regexp.MustCompile(`<[^>]*>`)
	mdLinkPat           = regexp.MustCompile(`\[(.*?)\]\([^\)]*\)`)
	mdFormatPat         = regexp.MustCompile(`[*_` + "`" + `~]`)
	slugRepeatPat       = regexp.MustCompile(`-{2,}`)
	frontmatterBlockPat = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n?`)
	datePrefixPat       = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-`)
	fieldLinePat        = regexp.MustCompile(`(?m)^([A-Za-z][A-Za-z0-9_-]*):[ \t]+(\S.*)$`)

	financePat  = regexp.MustCompile(`(?i)\b(bank\w*|finan\w*|fiduciary|ledger\w*)\b`)
	securityPat = regexp.MustCompile(`(?i)\b(security|dora|crypto\w*|audit\w*)\b`)
	aiPat       = regexp.MustCompile(`(?i)\b(ai|agent\w*|model\w*|gemma\w*)\b`)
)

// PartOfSet reports whether path belongs to a generated body/frontmatter/final
// trio, either by filename suffix or by sitting in a day-folder source/ or
// final/ directory.
func PartOfSet(path string) bool {
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if strings.HasSuffix(stem, "-body") || strings.HasSuffix(stem, "-final") || strings.HasSuffix(stem, "-frontmatter") {
		return true
	}
	dirName := filepath.Base(filepath.Dir(path))
	return dirName == "source" || dirName == "final"
}

// Metadata holds extracted article metadata fields used to build YAML frontmatter.
type Metadata struct {
	Title       string
	Subtitle    string
	Description string
	Excerpt     string
	Keywords    string
	Tags        string
	Category    string
	Slug        string
}

// Split divides a raw document into frontmatter (if present) and body content.
func Split(content string) (string, string) {
	content = strings.TrimSpace(content)
	if loc := frontmatterBlockPat.FindStringIndex(content); loc != nil && loc[0] == 0 {
		fm := content[loc[0]:loc[1]]
		body := strings.TrimSpace(content[loc[1]:])
		return fm, body
	}
	return "", content
}

// Combine joins a YAML frontmatter block (with --- delimiting lines) and a Markdown body.
func Combine(frontmatterYAML, body string) string {
	fm := strings.TrimSpace(frontmatterYAML)
	bd := strings.TrimSpace(body)
	if fm == "" {
		return bd
	}
	if !strings.HasPrefix(fm, "---") {
		fm = "---\n\n" + fm
	}
	// Closed means the last LINE is a delimiter, not merely that the last
	// characters happen to be dashes.
	if fm != "---" && !strings.HasSuffix(fm, "\n---") {
		fm += "\n---"
	}
	return fm + "\n\n" + bd + "\n"
}

// ExtractMetadata parses the Markdown body and extracts metadata for frontmatter generation.
func ExtractMetadata(markdown string) Metadata {
	_, body := Split(markdown)

	// 1. Title
	title := "Untitled Article"
	if m := titlePat.FindStringSubmatch(body); len(m) >= 2 {
		title = cleanText(m[1])
	}

	// 2. Subtitle / Thesis: the deck paragraph is the standfirst when present,
	// otherwise the first standalone bold line.
	subtitle := ""
	if m := deckPat.FindStringSubmatch(body); len(m) >= 2 {
		subtitle = cleanText(m[1])
	}
	if subtitle == "" {
		if m := boldLinePat.FindStringSubmatch(body); len(m) >= 2 {
			candidate := cleanText(m[1])
			if !strings.HasPrefix(strings.ToLower(candidate), "author") &&
				!strings.HasPrefix(strings.ToLower(candidate), "executive summary") {
				subtitle = candidate
			}
		}
	}

	// 3. Description & Excerpt
	desc := ""
	if m := tldrPat.FindStringSubmatch(body); len(m) >= 2 {
		desc = cleanText(m[1])
	}
	if desc == "" {
		if m := execSummaryPat.FindStringSubmatch(body); len(m) >= 2 {
			lines := strings.Split(m[1], "\n")
			var items []string
			for _, l := range lines {
				l = strings.TrimSpace(l)
				l = strings.TrimPrefix(l, ">")
				l = strings.TrimPrefix(l, "-")
				l = strings.TrimPrefix(l, "*")
				l = cleanText(l)
				if l != "" {
					items = append(items, l)
				}
			}
			if len(items) > 0 {
				desc = strings.Join(items, " ")
			}
		}
	}
	if desc == "" && subtitle != "" {
		desc = subtitle
	}
	if desc == "" {
		desc = title
	}
	if len(desc) > 200 {
		desc = truncateAtWord(desc, 197) + "..."
	}

	excerpt := desc
	if subtitle != "" && len(subtitle) > len(desc) {
		excerpt = subtitle
	}
	if len(excerpt) > 220 {
		excerpt = truncateAtWord(excerpt, 217) + "..."
	}

	// 4. Keywords & Tags: title terms lead (they are topical by construction),
	// then the most frequent content words from the body. H2 headings are
	// deliberately not harvested — in practice they are sentence fragments,
	// not keywords.
	kwList := extractKeyTerms(title)
	kwList = append(kwList, extractKeyTerms(body)...)
	kwList = dedupeStrings(kwList)
	if len(kwList) > 12 {
		kwList = kwList[:12]
	}

	keywordsStr := strings.Join(kwList, ", ")
	tagsStr := keywordsStr

	// 5. Category
	cat := inferCategory(title, body)

	// 6. Slug
	slug := Slugify(title)

	return Metadata{
		Title:       title,
		Subtitle:    subtitle,
		Description: desc,
		Excerpt:     excerpt,
		Keywords:    keywordsStr,
		Tags:        tagsStr,
		Category:    cat,
		Slug:        slug,
	}
}

// parseFields extracts single-line "key: value" pairs from a frontmatter
// block, keeping values raw (quoting included) so they round-trip verbatim.
func parseFields(fm string) map[string]string {
	fields := make(map[string]string)
	for _, m := range fieldLinePat.FindAllStringSubmatch(fm, -1) {
		fields[m[1]] = strings.TrimRight(m[2], " \t")
	}
	return fields
}
