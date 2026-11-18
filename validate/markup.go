// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"html"
	"regexp"
	"strings"
)

// A saved article is published by a Markdown site generator, and Jekyll and
// kramdown pass raw HTML through untouched. The article's text is partly
// chosen by the source document — a claim's quote is verbatim by design — so
// whatever executes in a browser has to be stopped here rather than trusted to
// the writer. This is a denylist of what executes, not a ban on HTML: the
// house structure is itself raw HTML (the lead aside), and a ban on every tag
// would make the format unwritable.
//
// Every pattern is anchored and tried only where its first byte occurs. Errors
// runs on every draft and every retry, and a clean article has a handful of
// candidate bytes, so the check costs a byte scan rather than a regex pass per
// construct over the whole text.
var (
	// unsafeTagPat names the elements that run script, load another document
	// or rewrite the page around the article. A closing tag counts as well as
	// an opening one, and so does a tag cut off at the end of the text, since
	// a browser will happily close it on whatever markup follows.
	unsafeTagPat = regexp.MustCompile(`^(?i)</?(script|iframe|frame|frameset|object|embed|applet|style|base|meta|link|form)(?:[\s/>]|$)`)
	// kramdownIALPat is kramdown's inline attribute list, `{: key="value"}`,
	// which sets arbitrary attributes — an event handler among them — on the
	// element before it without any HTML in sight.
	kramdownIALPat = regexp.MustCompile(`^\{:([^{}\n]*)\}`)
	// The three places Markdown itself puts a URL: an inline link or image
	// destination (optionally in angle brackets, which may hold spaces), a
	// reference definition, and an autolink.
	//
	// A destination is read only up to a bracket. No bracket can appear in a
	// scheme or in the entity that spells one, so nothing that decides the
	// verdict is lost, and a run of "](" is read once rather than once per
	// bracket.
	inlineDestPat = regexp.MustCompile(`^\]\(\s*(<[^>\n\[\]]*>|[^\s()\[\]]*)`)
	refDefPat     = regexp.MustCompile(`^ {0,3}\[[^\]\n]+\]:[ \t]*(<[^>\n]*>|\S+)`)
	autolinkPat   = regexp.MustCompile(`^<([a-zA-Z][a-zA-Z0-9+.\-]*:[^\s<>]*)>`)
)

// urlAttributes are the attributes a browser resolves as a URL. Only these are
// checked for a scheme: checking every value would flag a title that reads
// "data: see table 2".
var urlAttributes = map[string]bool{
	"href": true, "src": true, "srcset": true, "action": true, "formaction": true,
	"xlink:href": true, "poster": true, "background": true, "cite": true,
	"data": true, "longdesc": true, "ping": true,
}

// unsafeSchemes are the URL schemes that run code or carry a whole document.
// data: is refused outright, images included: draft never produces an image,
// so an inline one can only have come from the source or the model, and an
// SVG data URI is a document that runs script when it is opened directly.
var unsafeSchemes = []string{"javascript:", "vbscript:", "data:"}

// UnsafeMarkup lists the constructs in md that would run script or load
// foreign content once a Markdown site renders it: dangerous elements, event
// handler attributes (in HTML or a kramdown attribute list), and javascript:,
// vbscript: or data: URLs in a link, image, reference definition, autolink or
// URL attribute. Each kind is listed once, in the order first found, so a
// caller can compare two drafts. An empty result means none was found.
//
// Code spans and fences are not exempt. A renderer escapes them, but the
// article's text also feeds frontmatter and excerpts that strip the backticks,
// and an article that needs a live payload in code is not one draft writes.
func UnsafeMarkup(md string) []string {
	var f markupFindings
	for i := 0; i < len(md); i++ {
		j := strings.IndexAny(md[i:], "<]{[")
		if j < 0 {
			break
		}
		i += j
		f.at(md, i)
	}
	return f.list
}

// markupFindings collects each kind of finding once, in first-seen order. A
// clean article finds nothing, so nothing is allocated.
//
// Every candidate byte is examined, even one inside a construct already read:
// a '<' in a quoted attribute value is not a tag to a browser, but a Markdown
// renderer that does not take the outer tag for HTML will emit the inner one.
type markupFindings struct {
	list []string
}

func (f *markupFindings) add(kind string) {
	for _, k := range f.list {
		if k == kind {
			return
		}
	}
	f.list = append(f.list, kind)
}

// at checks the constructs that can start at md[i].
func (f *markupFindings) at(md string, i int) {
	rest := md[i:]
	switch rest[0] {
	case '<':
		f.tag(rest)
		f.submatch(autolinkPat, rest, f.url)
	case ']':
		f.submatch(inlineDestPat, rest, f.url)
	case '{':
		f.submatch(kramdownIALPat, rest, func(s string) { f.attributes(s) })
	case '[':
		if start := lineStartBefore(md, i); start >= 0 {
			f.submatch(refDefPat, md[start:], f.url)
		}
	}
}

// lineStartBefore returns where the line holding md[i] starts when only up to
// three spaces precede i on it, which is where a reference definition may
// open, or -1. It looks back no further than that, so a long line of brackets
// costs nothing per bracket.
func lineStartBefore(md string, i int) int {
	k := i
	for k > 0 && i-k < 3 && md[k-1] == ' ' {
		k--
	}
	if k == 0 || md[k-1] == '\n' {
		return k
	}
	return -1
}

// tag checks a tag opening at s[0] == '<'.
//
// The name and the attribute names stop at the next '<'. A browser would
// carry on, but whatever follows that '<' is read again as a tag of its own,
// so the attributes are all still seen, and a run of '<' costs linear time
// rather than quadratic.
func (f *markupFindings) tag(s string) {
	if m := unsafeTagPat.FindStringSubmatchIndex(s); m != nil {
		f.add("<" + strings.ToLower(s[m[2]:m[3]]) + ">")
	}
	if len(s) < 2 || !isASCIILetter(s[1]) {
		return
	}
	name := 1
	for name < len(s) && !endsAttrName(s[name]) {
		name++
	}
	f.attributes(s[name:])
}

// submatch runs an anchored pattern on s and hands its first group to check.
func (f *markupFindings) submatch(pat *regexp.Regexp, s string, check func(string)) {
	if m := pat.FindStringSubmatchIndex(s); m != nil {
		check(s[m[2]:m[3]])
	}
}

// attributes reads attributes the way a browser's tokeniser does, up to the
// '>' that closes the tag. It is written out by hand rather than as a pattern
// because the lead aside puts a tag in every article, and a regex match there
// allocated on every validation of every draft.
func (f *markupFindings) attributes(s string) {
	for i := 0; i < len(s) && s[i] != '>' && s[i] != '<'; {
		if isAttrSeparator(s[i]) {
			i++
			continue
		}
		i += f.attribute(s[i:])
	}
}

// attribute reads one name and its optional value from the start of s, checks
// them, and returns how many bytes it read: always at least one, because a
// browser takes even a leading '=' as part of a name.
func (f *markupFindings) attribute(s string) int {
	n := 1
	for n < len(s) && !endsAttrName(s[n]) && s[n] != '=' {
		n++
	}
	j := skipHTMLSpace(s, n)
	if j >= len(s) || s[j] != '=' {
		return n
	}
	j = skipHTMLSpace(s, j+1)
	v := attrValueLen(s[j:])
	f.check(strings.ToLower(s[:n]), s[j:j+v])
	return j + v
}

// check judges one attribute. Only a name with a value can act, so a bare
// word in an attribute list is never reported.
func (f *markupFindings) check(name, value string) {
	if isHandlerName(name) {
		f.add(name + "=")
	}
	if urlAttributes[name] {
		f.url(value)
	}
}

// attrValueLen is the length of the attribute value at the start of s: a
// quoted value up to its closing quote, or to the end of the text when the
// quote is never closed, as a browser reads it; an unquoted one up to
// whitespace, '>' or, for the reason tag gives, '<' (which no scheme holds).
func attrValueLen(s string) int {
	if s != "" && (s[0] == '"' || s[0] == '\'') {
		if end := strings.IndexByte(s[1:], s[0]); end >= 0 {
			return end + 2
		}
		return len(s)
	}
	n := 0
	for n < len(s) && !isHTMLSpace(s[n]) && s[n] != '>' && s[n] != '<' {
		n++
	}
	return n
}

// endsAttrName reports a byte that ends a tag or attribute name.
func endsAttrName(c byte) bool { return c == '>' || c == '<' || isAttrSeparator(c) }

func skipHTMLSpace(s string, i int) int {
	for i < len(s) && isHTMLSpace(s[i]) {
		i++
	}
	return i
}

func isHTMLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\f' || c == '\r'
}

// isHandlerName reports an event handler attribute: "on" and then letters,
// which is the shape of every one a browser defines. A name holding anything
// else is a stray token, not a handler.
func isHandlerName(name string) bool {
	if len(name) < 3 || !strings.HasPrefix(name, "on") {
		return false
	}
	for i := 2; i < len(name); i++ {
		if !isASCIILetter(name[i]) {
			return false
		}
	}
	return true
}

// isAttrSeparator includes '/', which a browser skips between attributes.
func isAttrSeparator(c byte) bool { return c == '/' || isHTMLSpace(c) }

func (f *markupFindings) url(raw string) {
	u := normaliseURL(raw)
	for _, s := range unsafeSchemes {
		if strings.HasPrefix(u, s) {
			f.add(s + " URL")
		}
	}
}

// normaliseURL reduces a URL to what a browser would take as its scheme.
// Entities are decoded because both Markdown and HTML attributes decode them,
// so "&#106;avascript:" is javascript:. Whitespace and controls are dropped
// because browsers strip them from a URL before reading the scheme, and
// backslashes because Markdown uses them to escape the colon. Quotes and angle
// brackets are the delimiters the patterns above leave on the value.
func normaliseURL(raw string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		switch {
		case r <= ' ', r == '\\', r == '"', r == '\'', r == '<', r == '>':
			return -1
		}
		return r
	}, html.UnescapeString(raw)))
}
