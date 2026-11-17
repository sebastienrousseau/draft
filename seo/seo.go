// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package seo checks a finished article against the guidance Google Search
// Central publishes for page content and metadata.
//
// Every rule enforces a sentence from that guidance and names the page it
// comes from. Just as deliberately, nothing Google says it ignores is
// checked: there is no title or description length limit ("there's no limit
// on how long a <title> element can be"), no word count, no keyword density,
// no meta keywords and no heading order. A rule invented from folklore would
// push drafts away from the guidance, not towards it.
package seo

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/sebastienrousseau/draft/frontmatter"
)

// Severity says how firmly the guidance is contradicted.
type Severity string

const (
	// Error marks a departure from something the guidance asks for outright.
	Error Severity = "error"
	// Warning marks something the guidance asks an author to consider.
	Warning Severity = "warning"
)

// The Google Search Central pages the rules come from.
const (
	DocStarter = "https://developers.google.com/search/docs/fundamentals/seo-starter-guide"
	DocTitle   = "https://developers.google.com/search/docs/appearance/title-link"
	DocSnippet = "https://developers.google.com/search/docs/appearance/snippet"
	DocHelpful = "https://developers.google.com/search/docs/fundamentals/creating-helpful-content"
	DocFAQ     = "https://developers.google.com/search/docs/appearance/structured-data/faqpage"
)

// Finding is one way an article departs from the guidance.
type Finding struct {
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Source   string   `json:"source"`
}

// Article is the part of a final article the rules read: its frontmatter
// fields, unquoted, and its Markdown body.
type Article struct {
	Path   string
	Fields map[string]string
	Body   string
}

var fieldLine = regexp.MustCompile(`(?m)^([A-Za-z][A-Za-z0-9_-]*):[ \t]+(\S.*?)[ \t]*$`)

// Parse reads a final article (frontmatter followed by body). A document
// with no frontmatter yields an Article with no fields, which the rules then
// report as missing a title, description and byline.
func Parse(path, content string) Article {
	fm, body := frontmatter.Split(content)
	fields := make(map[string]string)
	for _, m := range fieldLine.FindAllStringSubmatch(fm, -1) {
		fields[m[1]] = unquote(m[2])
	}
	return Article{Path: path, Fields: fields, Body: body}
}

// unquote turns a YAML scalar as draft writes it into its value. Draft
// writes double-quoted strings; a single-quoted or bare value is accepted so
// a hand-edited file is read the way its author meant.
func unquote(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		if s, err := strconv.Unquote(v); err == nil {
			return s
		}
		return v[1 : len(v)-1]
	}
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return strings.ReplaceAll(v[1:len(v)-1], "''", "'")
	}
	return v
}

// HasErrors reports whether any finding is an Error.
func HasErrors(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == Error {
			return true
		}
	}
	return false
}

// Check runs every per-article rule on a.
func Check(a Article) []Finding {
	return CheckSet([]Article{a})[0]
}

// CheckSet runs every per-article rule on each article, then the rules that
// only a set can break: Google asks for a title and a description "unique to
// the page", so two articles sharing one is a finding on both. The result
// has one slice per article, in the order given.
func CheckSet(articles []Article) [][]Finding {
	out := make([][]Finding, len(articles))
	for i, a := range articles {
		for _, r := range rules {
			out[i] = append(out[i], r(a)...)
		}
	}
	duplicates(articles, out, "title", DocTitle)
	duplicates(articles, out, "description", DocSnippet)
	return out
}

var rules = []func(Article) []Finding{
	checkTitle, checkDescription, checkImages, checkLinks, checkHeadings,
	checkByline, checkDisclosure, checkCanonical, checkStructuredData,
}

func field(a Article, key string) string {
	return strings.TrimSpace(a.Fields[key])
}

func finding(rule string, sev Severity, source, msg string) []Finding {
	return []Finding{{Rule: rule, Severity: sev, Message: msg, Source: source}}
}

// duplicates flags every article whose key field matches another's.
func duplicates(articles []Article, out [][]Finding, key, source string) {
	seen := make(map[string]int)
	for i, a := range articles {
		v := strings.ToLower(field(a, key))
		if v == "" {
			continue
		}
		j, dup := seen[v]
		if !dup {
			seen[v] = i
			continue
		}
		msg := "the " + key + " is the same as in "
		out[i] = append(out[i], finding(key+"-duplicate", Error, source, msg+articles[j].Path)...)
		out[j] = append(out[j], finding(key+"-duplicate", Error, source, msg+a.Path)...)
	}
}
