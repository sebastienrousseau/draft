// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package validate

import (
	"regexp"
	"strings"
	"testing"
)

// unsafeKind is every shape UnsafeMarkup may report. --review keys its
// comparison on these, so anything else is a bug in the reader.
var unsafeKind = regexp.MustCompile(`^(<(script|iframe|frame|frameset|object|embed|applet|style|base|meta|link|form)>|on[a-z]+=|(javascript|vbscript|data): URL)$`)

// FuzzUnsafeMarkup drives the markup reader with arbitrary text. It must never
// panic, must report only the shapes it documents, and no surrounding text may
// hide a script tag: whatever came before, a browser still sees one.
func FuzzUnsafeMarkup(f *testing.F) {
	f.Add(`<aside class="post-lead"><p>TL;DR.</p></aside>`, 0)
	f.Add(`[a](javascript:alert(1)) <img src=x onerror=y>`, 3)
	f.Add("[r]: <data:text/html,x>\n{: onclick=\"x\"}", 7)
	f.Add(`<a title="unterminated`, 4)
	f.Add(`<a href='&#106;ava&Tab;script:x'>`, 12)
	f.Add("", 0)

	f.Fuzz(func(t *testing.T, md string, at int) {
		got := UnsafeMarkup(md)
		seen := map[string]bool{}
		for _, k := range got {
			if !unsafeKind.MatchString(k) {
				t.Errorf("UnsafeMarkup(%q) reported an undocumented kind %q", md, k)
			}
			if seen[k] {
				t.Errorf("UnsafeMarkup(%q) reported %q twice", md, k)
			}
			seen[k] = true
		}
		if at < 0 {
			at = -at
		}
		if len(md) > 0 {
			at %= len(md) + 1
		} else {
			at = 0
		}
		spliced := md[:at] + "<script>" + md[at:]
		found := false
		for _, k := range UnsafeMarkup(spliced) {
			found = found || k == "<script>"
		}
		if !found {
			t.Errorf("a script tag spliced into %q at %d went unreported", md, at)
		}
		if strings.Join(UnsafeMarkup(md), "|") != strings.Join(got, "|") {
			t.Errorf("UnsafeMarkup(%q) is not deterministic", md)
		}
	})
}
