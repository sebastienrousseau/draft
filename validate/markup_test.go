// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"strings"
	"testing"
	"time"
)

// A Jekyll or kramdown site renders raw HTML in Markdown as HTML, so anything
// here that reaches a saved article runs in a reader's browser. A verbatim
// quote is the easy route in: the source document chooses its words.
func TestUnsafeMarkupIsAViolation(t *testing.T) {
	for _, tc := range []struct{ name, md, want string }{
		{"script tag", `He wrote <script>alert(1)</script> in the abstract.`, "<script>"},
		{"script tag in capitals", `<SCRIPT SRC=//x.example/a.js></SCRIPT>`, "<script>"},
		{"closing tag alone", `text </script> more`, "<script>"},
		{"iframe", `<iframe src="https://x.example"></iframe>`, "<iframe>"},
		{"object", `<object data="x.swf"></object>`, "<object>"},
		{"embed", `<embed src="x.swf">`, "<embed>"},
		{"style", `<style>body{display:none}</style>`, "<style>"},
		{"base", `<base href="https://x.example/">`, "<base>"},
		{"meta refresh", `<meta http-equiv="refresh" content="0;url=https://x.example">`, "<meta>"},
		{"event handler", `<img src=x onerror=alert(1)>`, "onerror="},
		{"event handler quoted after quote", `<a href="#"onclick="x()">a</a>`, "onclick="},
		{"event handler on a later line", "<p\n  onmouseover=\"x()\">a</p>", "onmouseover="},
		{"kramdown attribute list", "A paragraph.\n{: onmouseover=\"alert(1)\"}", "onmouseover="},
		{"javascript link", `[read more](javascript:alert(1))`, "javascript: URL"},
		{"javascript link with spaces", `[read more]( JavaScript:alert(1) )`, "javascript: URL"},
		{"javascript in angle destination", `[a](<java script:alert(1)>)`, "javascript: URL"},
		{"entity-encoded scheme", `[a](&#106;avascript:alert(1))`, "javascript: URL"},
		{"entity without semicolon", `[a](&#x6A;ava&#x09;script:alert(1))`, "javascript: URL"},
		{"backslash-escaped colon", `[a](javascript\:alert(1))`, "javascript: URL"},
		{"vbscript link", `[a](vbscript:msgbox(1))`, "vbscript: URL"},
		{"data link", `[a](data:text/html;base64,PHNjcmlwdD4=)`, "data: URL"},
		{"data image", `![a](data:image/svg+xml;base64,PHN2Zz4=)`, "data: URL"},
		{"reference definition", "See [the paper][p].\n\n[p]: javascript:alert(1)", "javascript: URL"},
		{"autolink", `<javascript:alert(1)>`, "javascript: URL"},
		{"href attribute", `<a href="javascript:alert(1)">a</a>`, "javascript: URL"},
		{"href attribute with entity tab", `<a href="java&Tab;script:alert(1)">a</a>`, "javascript: URL"},
		{"unquoted src", `<img src=javascript:alert(1)>`, "javascript: URL"},
		{"svg xlink", `<svg><a xlink:href='javascript:alert(1)'>a</a></svg>`, "javascript: URL"},
		{"handler after a quoted '<'", `<a title="<b" onclick="x()">a</a>`, "onclick="},
		{"handler after a stray '<'", `<a b <img src=x onerror=y>`, "onerror="},
		{"link inside an unclosed tag", `<x it's here and [a](javascript:alert(1)) isn't`, "javascript: URL"},
		{"tag hidden in an attribute", `<a title='<script>'>`, "<script>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := UnsafeMarkup(tc.md)
			if !containsString(got, tc.want) {
				t.Errorf("UnsafeMarkup(%q) = %v, want it to name %q", tc.md, got, tc.want)
			}
			var msg string
			for _, e := range Errors(tc.md) {
				if strings.HasPrefix(e, "contains unsafe markup: ") {
					msg = e
				}
			}
			if !strings.Contains(msg, tc.want) {
				t.Errorf("Errors did not report %q as unsafe markup: %q", tc.want, msg)
			}
		})
	}
}

// The check is a denylist of what executes, not a ban on HTML or on words that
// look like schemes. The house structure itself is raw HTML, and research prose
// says "data:" and "JavaScript:" all the time.
func TestSafeMarkupIsNotAViolation(t *testing.T) {
	for _, tc := range []struct{ name, md string }{
		{"the house aside", `<aside class="post-lead" aria-label="Article summary"><p class="post-lead-tldr"><strong>TL;DR.</strong> A look.</p></aside>`},
		{"comparison", `The effect held (p < 0.05) when n > 30.`},
		{"label with colon", `Training data: 5,000 samples. JavaScript: the language of the web.`},
		{"https link", `[the paper](https://arxiv.org/abs/2401.00001 "Title")`},
		{"relative link", `[section](#results) and [file](./a.md)`},
		{"mailto autolink", `<mailto:a@example.org> and <https://example.org>`},
		{"escaped tag", `The payload was &lt;script&gt;alert(1)&lt;/script&gt;.`},
		{"word starting with on", `<p class="online">Done.</p>`},
		{"attribute named like a handler", `<abbr title="one=1">x</abbr>`},
		{"prose about scripts", `The script ran on the iframe-free page; a description of style.`},
		{"ordinary braces", `The set {a, b} and {: not an attribute list}.`},
		{"data in an https URL", `[a](https://example.org/data:raw)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := UnsafeMarkup(tc.md); len(got) != 0 {
				t.Errorf("UnsafeMarkup(%q) = %v, want nothing", tc.md, got)
			}
		})
	}
}

// One entry per kind of construct, in first-seen order, so --review can tell
// an edit that adds one from an article that already had one.
func TestUnsafeMarkupReportsEachKindOnce(t *testing.T) {
	md := `<script>a</script> <script>b</script> [x](javascript:1) [y](javascript:2) <img onerror=x>`
	got := UnsafeMarkup(md)
	want := []string{"<script>", "javascript: URL", "onerror="}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("UnsafeMarkup = %v, want %v", got, want)
	}
}

// Every '<', ']', '[' and '{' starts a read, so a run of them must not make
// each read cover the rest of the text. These took up to seven seconds each
// before the reads were bounded; the limit is generous so a slow, race-enabled
// CI runner still passes.
func TestUnsafeMarkupStaysLinear(t *testing.T) {
	for _, s := range []string{
		strings.Repeat("<a", 15000), strings.Repeat("](", 15000), strings.Repeat(`<a x="`, 5000),
		strings.Repeat("[", 30000), strings.Repeat("{:", 15000), strings.Repeat("<a:", 10000),
		strings.Repeat("<a b ", 6000), strings.Repeat("<a x=", 6000),
	} {
		start := time.Now()
		UnsafeMarkup(s)
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("UnsafeMarkup(%.12q...) took %v", s, d)
		}
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
