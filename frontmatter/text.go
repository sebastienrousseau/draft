// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package frontmatter

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Slugify converts text into a URL-friendly slug.
func Slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	out = slugRepeatPat.ReplaceAllString(out, "-")
	if len(out) > 90 {
		out = strings.Trim(out[:90], "-")
	}
	if out == "" {
		return "draft-article"
	}
	return out
}

// readCapped reads a file, refusing anything larger than limit rather than
// pulling it all into memory first.
func readCapped(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	if info, statErr := f.Stat(); statErr == nil && info.Size() > limit {
		return nil, fmt.Errorf("%s is %d bytes; the limit is %d", filepath.Base(path), info.Size(), limit)
	}
	// LimitReader guards the case where Stat lied or the file grew after it.
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s exceeds the %d byte limit", filepath.Base(path), limit)
	}
	return b, nil
}

// quoteYAML renders s as a YAML double-quoted scalar.
//
// Control characters are stripped rather than escaped: a double-quoted scalar
// may not carry a raw control character, and a title or description that
// somehow contains one has no business carrying it into published frontmatter.
// Whitespace is folded to single spaces by strings.Fields, which already
// covers tab and carriage return.
func quoteYAML(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == '\r' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.Join(strings.Fields(s), " ")
	return `"` + s + `"`
}

func cleanText(s string) string {
	s = htmlTagPat.ReplaceAllString(s, "")
	s = mdLinkPat.ReplaceAllString(s, "$1")
	s = mdFormatPat.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

func truncateAtWord(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	sub := s[:maxLen]
	if lastIdx := strings.LastIndex(sub, " "); lastIdx > 0 {
		return sub[:lastIdx]
	}
	// No word boundary: back off to a rune boundary so the cut stays valid UTF-8.
	for len(sub) > 0 && !utf8.ValidString(sub) {
		sub = sub[:len(sub)-1]
	}
	return sub
}

func extractKeyTerms(body string) []string {
	words := strings.Fields(cleanText(body))
	counts := make(map[string]int)
	order := make([]string, 0)

	for _, w := range words {
		wLower := strings.ToLower(w)
		wClean := strings.Trim(wLower, ".,;:!?()[]{}'\"")
		if len(wClean) > 3 && !stopWords[wClean] {
			if counts[wClean] == 0 {
				order = append(order, wClean)
			}
			counts[wClean]++
		}
	}

	sort.SliceStable(order, func(i, j int) bool {
		return counts[order[i]] > counts[order[j]]
	})

	if len(order) > 8 {
		return order[:8]
	}
	return order
}

// stopWords are function words, auxiliaries, and generic filler that make bad
// keywords no matter how often they appear.
var stopWords = func() map[string]bool {
	list := []string{
		// articles, conjunctions, prepositions
		"a", "an", "the", "and", "or", "but", "nor", "so", "yet",
		"in", "on", "at", "to", "for", "of", "with", "by", "from", "up", "down",
		"about", "into", "onto", "over", "under", "after", "before", "between",
		"through", "during", "against", "within", "without", "across", "around",
		"among", "toward", "towards", "upon", "off", "out", "above", "below",
		// pronouns and determiners
		"it", "its", "this", "that", "these", "those", "they", "them", "their",
		"theirs", "there", "here", "which", "whose", "what", "when", "where",
		"who", "whom", "whichever", "some", "such", "each", "every", "both",
		"other", "another", "any", "all", "none", "few", "several", "your",
		"yours", "ours", "his", "hers",
		// auxiliaries and modals
		"is", "are", "was", "were", "be", "been", "being", "have", "has", "had",
		"do", "does", "did", "done", "will", "would", "shall", "should", "can",
		"could", "may", "might", "must", "wont", "cant", "dont", "isnt", "arent",
		// generic verbs
		"make", "makes", "made", "take", "takes", "taken", "took", "give",
		"gives", "given", "gave", "get", "gets", "got", "goes", "went", "come",
		"comes", "came", "use", "uses", "used", "using", "turn", "turns",
		"turned", "mean", "means", "meant", "need", "needs", "needed", "want",
		"wants", "say", "says", "said", "see", "sees", "seen", "know", "knows",
		"known", "improve", "process", "provide", "provides", "keep", "keeps",
		"show", "shows", "shown", "find", "finds", "found", "look", "looks",
		// adverbs, qualifiers, connectives
		"not", "only", "same", "very", "much", "many", "more", "most", "less",
		"least", "than", "then", "also", "just", "like", "even", "still",
		"already", "often", "always", "never", "again", "once", "twice",
		"however", "therefore", "instead", "rather", "because", "since",
		"while", "whether", "though", "although", "almost", "enough", "quite",
		"really", "well", "how", "why", "yes",
		// generic nouns
		"thing", "things", "way", "ways", "part", "parts", "kind", "kinds",
		"lot", "lots", "case", "cases", "point", "points", "fact", "facts",
		"time", "times", "year", "years", "day", "days", "one", "ones", "two",
		"three", "first", "second", "third", "last", "next", "new", "old",
	}
	m := make(map[string]bool, len(list))
	for _, w := range list {
		m[w] = true
	}
	return m
}()

func dedupeStrings(slice []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range slice {
		item = strings.TrimSpace(item)
		if item != "" && !seen[strings.ToLower(item)] {
			seen[strings.ToLower(item)] = true
			result = append(result, item)
		}
	}
	return result
}

// inferCategory scores each category by how often its terms appear, so a
// dominant theme beats a passing mention. Ties keep the listed priority;
// no matches at all fall back to Technology.
func inferCategory(title, body string) string {
	combined := title + " " + body
	best, bestN := "Technology", 0
	for _, c := range []struct {
		name string
		pat  *regexp.Regexp
	}{
		{"Finance", financePat},
		{"Security", securityPat},
		{"AI", aiPat},
	} {
		if n := len(c.pat.FindAllString(combined, -1)); n > bestN {
			best, bestN = c.name, n
		}
	}
	return best
}
