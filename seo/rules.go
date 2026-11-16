// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package seo

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// genericTitles are the vague labels the title-link guidance names, plus the
// placeholders a draft carries before anyone has titled it.
var genericTitles = map[string]bool{
	"home": true, "profile": true, "untitled": true, "draft": true,
	"article": true, "new article": true, "untitled article": true, "post": true,
}

func checkTitle(a Article) []Finding {
	t := field(a, "title")
	switch {
	case t == "":
		return finding("title-missing", Error, DocTitle, "the article has no title")
	case genericTitles[strings.ToLower(t)] || strings.EqualFold(t, field(a, "name")):
		return finding("title-boilerplate", Error, DocTitle,
			fmt.Sprintf("the title %q does not describe this page", t))
	}
	// Three uses of one word in a title is what the guidance calls
	// "repeating the same words or phrases"; twice can be legitimate
	// ("Payments after payments regulation").
	if w := repeatedWord(t, 3); w != "" {
		return finding("title-repeated-word", Error, DocTitle,
			fmt.Sprintf("the title repeats %q; Google treats repetition as keyword stuffing", w))
	}
	return nil
}

func checkDescription(a Article) []Finding {
	d := field(a, "description")
	switch {
	case d == "":
		return finding("description-missing", Error, DocSnippet, "the article has no meta description")
	case len(strings.Fields(d)) < 3:
		return finding("description-too-short", Error, DocSnippet,
			"the description is one or two words; Google calls that too short to be useful")
	case isKeywordList(d):
		return finding("description-keyword-list", Error, DocSnippet,
			"the description is a list of keywords, not a summary of the page")
	case strings.HasSuffix(d, "...") || strings.HasSuffix(d, "…"):
		return finding("description-truncated", Warning, DocSnippet,
			"the description stops mid-sentence; Google asks for a summary of the whole page")
	case strings.EqualFold(d, field(a, "title")):
		return finding("description-repeats-title", Warning, DocSnippet,
			"the description repeats the title instead of summarising the page")
	}
	return nil
}

// isKeywordList recognises "long strings of keywords": no sentence ends,
// and every comma-separated part is a short phrase.
func isKeywordList(s string) bool {
	if strings.ContainsAny(s, ".!?") {
		return false
	}
	parts := strings.Split(s, ",")
	if len(parts) < 4 {
		return false
	}
	for _, p := range parts {
		if len(strings.Fields(p)) > 3 {
			return false
		}
	}
	return true
}

var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "the": true, "of": true, "to": true,
	"in": true, "on": true, "for": true, "is": true, "are": true, "with": true,
	"by": true, "at": true, "or": true, "from": true, "as": true, "vs": true,
}

// repeatedWord returns the first word of s, ignoring case and stop words,
// that occurs at least n times, or "".
func repeatedWord(s string, n int) string {
	counts := make(map[string]int)
	for _, w := range strings.FieldsFunc(strings.ToLower(s), notWordRune) {
		if stopWords[w] {
			continue
		}
		counts[w]++
		if counts[w] == n {
			return w
		}
	}
	return ""
}

func notWordRune(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

var (
	mdImage  = regexp.MustCompile(`!\[([^\]]*)\]\(`)
	htmlImg  = regexp.MustCompile(`(?i)<img\b[^>]*>`)
	htmlAlt  = regexp.MustCompile(`(?i)\balt\s*=\s*("[^"]*\S[^"]*"|'[^']*\S[^']*')`)
	fenceTag = regexp.MustCompile("^\\s*(```|~~~)")
)

// prose is the body without fenced code, where an image or a heading is an
// example rather than part of the page.
func prose(body string) string {
	var b strings.Builder
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		if fenceTag.MatchString(line) {
			inFence = !inFence
			continue
		}
		if !inFence {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func checkImages(a Article) []Finding {
	var out []Finding
	for _, pair := range [][2]string{{"image", "image_alt"}, {"banner", "banner_alt"}} {
		if field(a, pair[0]) != "" && field(a, pair[1]) == "" {
			out = append(out, finding("image-alt-missing", Error, DocStarter,
				fmt.Sprintf("%s is set but %s is empty", pair[0], pair[1]))...)
		}
	}
	text := prose(a.Body)
	missing := 0
	for _, m := range mdImage.FindAllStringSubmatch(text, -1) {
		if strings.TrimSpace(m[1]) == "" {
			missing++
		}
	}
	for _, tag := range htmlImg.FindAllString(text, -1) {
		if !htmlAlt.MatchString(tag) {
			missing++
		}
	}
	if missing > 0 {
		out = append(out, finding("image-alt-missing", Error, DocStarter,
			fmt.Sprintf("%d image(s) in the body have no alt text", missing))...)
	}
	return out
}

var (
	// mdLink skips images: a link is a "[" not preceded by "!".
	mdLink = regexp.MustCompile(`(?:^|[^!])\[([^\]]+)\]\(\s*<?([^)\s>]+)`)
	// genericAnchors is link text that tells a reader nothing about the
	// destination, the opposite of the descriptive anchor text Google asks for.
	genericAnchors = map[string]bool{
		"here": true, "click here": true, "this": true, "link": true, "this link": true,
		"read more": true, "more": true, "this page": true, "learn more": true, "website": true,
	}
)

func checkLinks(a Article) []Finding {
	var out []Finding
	for _, m := range mdLink.FindAllStringSubmatch(prose(a.Body), -1) {
		anchor := strings.TrimSpace(strings.TrimFunc(m[1], notWordRune))
		if f := anchorFinding(anchor, m[2]); f != nil {
			out = append(out, f...)
		}
	}
	return out
}

func anchorFinding(anchor, target string) []Finding {
	lower := strings.ToLower(anchor)
	if genericAnchors[lower] || strings.HasPrefix(lower, "http") || strings.HasPrefix(lower, "www.") {
		return finding("link-generic-text", Warning, DocStarter,
			fmt.Sprintf("link text %q does not describe %s", anchor, target))
	}
	// A title-length anchor names a specific work; pointing it at a site's
	// front page leaves the claim it supports unverifiable, which the
	// helpful-content guidance counts against trust.
	if len(strings.Fields(anchor)) >= 4 && isSiteRoot(target) {
		return finding("link-to-site-root", Warning, DocHelpful,
			fmt.Sprintf("%q cites a specific work but links to the home page %s", anchor, target))
	}
	return nil
}

func isSiteRoot(target string) bool {
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		return false
	}
	return (u.Path == "" || u.Path == "/") && u.RawQuery == "" && u.Fragment == ""
}

func checkHeadings(a Article) []Finding {
	h1 := 0
	for _, line := range strings.Split(prose(a.Body), "\n") {
		if strings.HasPrefix(line, "# ") {
			h1++
		}
	}
	if h1 > 1 {
		return finding("heading-multiple-h1", Warning, DocTitle,
			fmt.Sprintf("the body has %d top-level headings; Google looks for one main title", h1))
	}
	return nil
}

func checkByline(a Article) []Finding {
	if field(a, "author") == "" {
		return finding("byline-missing", Error, DocHelpful,
			"no author: Google asks that it be self-evident who wrote the content")
	}
	return nil
}

func checkDisclosure(a Article) []Finding {
	if field(a, "draft_engine") == "" && field(a, "draft_model") == "" {
		return finding("disclosure-missing", Warning, DocHelpful,
			"nothing records that AI helped write this; Google asks that automation be self-evident where readers would expect it")
	}
	return nil
}

func checkCanonical(a Article) []Finding {
	u, p := field(a, "url"), field(a, "permalink")
	switch {
	case u == "" && p == "":
		return finding("canonical-missing", Error, DocStarter,
			"neither url nor permalink is set, so the page has no single canonical URL")
	case u != "" && p != "" && !strings.EqualFold(strings.TrimSuffix(u, "/"), strings.TrimSuffix(p, "/")):
		return finding("canonical-conflict", Error, DocStarter,
			fmt.Sprintf("url %s and permalink %s disagree; Google asks for one URL per piece of content", u, p))
	}
	return nil
}

func checkStructuredData(a Article) []Finding {
	s := field(a, "schema")
	switch {
	case s == "":
		return finding("structured-data-missing", Warning, DocStarter,
			"no schema type is declared, so the page is not eligible for rich results")
	case strings.Contains(strings.ToLower(s), "faqpage"):
		return finding("structured-data-faq-retired", Warning, DocFAQ,
			"FAQPage is declared, but Google stopped showing FAQ rich results on 7 May 2026")
	}
	return nil
}
