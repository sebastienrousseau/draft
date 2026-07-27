// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package frontmatter extracts metadata from article Markdown and generates YAML
// frontmatter, as well as splitting and combining frontmatter and article bodies.
package frontmatter

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

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

// Site holds the publisher identity stamped into generated frontmatter.
type Site struct {
	BaseURL       string // canonical site root, e.g. https://sebastienrousseau.com
	CDN           string // asset host, e.g. https://cloudcdn.pro
	Name          string // display name
	ShortName     string // slug-like identity used in asset paths
	Email         string
	TwitterHandle string
	Location      string
	MeasurementID string
	CopyrightFrom int // first year of the copyright range
}

// DefaultSite is the publisher identity used when Options.Site is nil.
var DefaultSite = Site{
	BaseURL:       "https://sebastienrousseau.com",
	CDN:           "https://cloudcdn.pro",
	Name:          "Sebastien Rousseau",
	ShortName:     "sebastienrousseau",
	Email:         "contact@sebastienrousseau.com",
	TwitterHandle: "@wwdseb",
	Location:      "London, UK",
	MeasurementID: "G-169G4ET5HQ",
	CopyrightFrom: 2025,
}

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
		fm = fm + "\n---"
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

// Options adjusts how Generate builds frontmatter beyond what the body provides.
type Options struct {
	// Date is used for all date-derived fields when the corresponding
	// existing field is absent. Zero means time.Now().
	Date time.Time
	// Slug overrides the title-derived slug for URL-shaped fields, so a
	// file's canonical URL follows its filename rather than its headline.
	Slug string
	// Existing holds field values from prior frontmatter (raw, as they
	// appeared after "key: "). They always win over generated values;
	// delete a field from the source frontmatter to have it regenerated.
	Existing map[string]string
	// Site is the publisher identity; nil means DefaultSite.
	Site *Site
}

// Generate constructs a YAML frontmatter string conforming to the standard schema.
func Generate(markdown string, date time.Time) string {
	return GenerateWithOptions(markdown, Options{Date: date})
}

// GenerateWithOptions constructs the standard frontmatter schema, preferring
// opts.Existing values over generated ones so curated metadata survives
// regeneration.
func GenerateWithOptions(markdown string, opts Options) string {
	meta := ExtractMetadata(markdown)
	if opts.Slug != "" {
		meta.Slug = opts.Slug
	}

	date := opts.Date
	if date.IsZero() {
		date = time.Now()
	}
	site := DefaultSite
	if opts.Site != nil {
		site = *opts.Site
	}

	// field renders one "key: value" line, keeping any existing value verbatim.
	field := func(key, generated string) string {
		if v, ok := opts.Existing[key]; ok {
			return key + ": " + v + "\n"
		}
		return key + ": " + quoteYAML(generated) + "\n"
	}

	dateStr := date.Format("2006-01-02")
	dateDisplay := date.Format("January 2, 2006")
	rssDate := date.Format("Mon, 02 Jan 2006 15:04:05 +0000")
	dateSlug := dateStr + "-" + meta.Slug
	fullURL := site.BaseURL + "/" + dateSlug
	rssURL := fullURL + "/rss.xml"
	bannerURL := site.CDN + "/stocks/images/" + meta.Slug + ".webp"
	bannerAlt := fmt.Sprintf("An illustration representing %s.", strings.ToLower(meta.Title))
	author := site.Email + " (" + site.Name + ")"
	logoURL := site.CDN + "/clients/" + site.ShortName + "/v1/logos/" + site.ShortName + ".svg"
	imageURL := site.CDN + "/stocks/images/" + site.ShortName + ".webp"
	copyright := fmt.Sprintf("© Copyright %d - %s. All rights reserved.", date.Year(), site.Name)
	if site.CopyrightFrom > 0 && site.CopyrightFrom < date.Year() {
		copyright = fmt.Sprintf("© Copyright %d - %d - %s. All rights reserved.", site.CopyrightFrom, date.Year(), site.Name)
	}

	var b strings.Builder
	b.WriteString("---\n\n")
	b.WriteString("# Front Matter (YAML)\n\n")
	b.WriteString(field("author", author))
	b.WriteString(field("banner_alt", bannerAlt))
	b.WriteString(field("banner_height", "1280"))
	b.WriteString(field("banner_width", "1920"))
	b.WriteString(field("banner", bannerURL))
	b.WriteString(field("cdn", site.CDN))
	b.WriteString(field("charset", "UTF-8"))
	b.WriteString(field("cname", strings.TrimPrefix(site.BaseURL, "https://")))
	b.WriteString(field("copyright", copyright))
	b.WriteString(field("date", dateDisplay))
	b.WriteString(field("description", meta.Description))
	b.WriteString(field("format-detection", "telephone=no"))
	b.WriteString(field("hreflang", "en"))
	b.WriteString(field("icon", logoURL))
	b.WriteString(field("id", fullURL))
	b.WriteString(field("image_alt", "Black and White Portrait of "+site.Name))
	b.WriteString(field("image_height", "162"))
	b.WriteString(field("image_width", "162"))
	b.WriteString(field("image", imageURL))
	b.WriteString(field("keywords", meta.Keywords))
	b.WriteString(field("language", "en-GB"))
	b.WriteString(field("last_reviewed", dateStr))
	b.WriteString(field("layout", "report"))
	b.WriteString(field("locale", "en_GB"))
	b.WriteString(field("logo_alt", "Logo for "+site.Name))
	b.WriteString(field("logo_height", "44"))
	b.WriteString(field("logo_width", "44"))
	b.WriteString(field("logo", logoURL))
	b.WriteString(field("menu", ""))
	b.WriteString(field("measurementID", site.MeasurementID))
	b.WriteString(field("name", site.Name))
	b.WriteString(field("permalink", fullURL))
	b.WriteString(field("rating", "general"))
	b.WriteString(field("referrer", "no-referrer"))
	b.WriteString(field("robots", "index, follow"))
	b.WriteString(field("schema", "FAQPage, Article"))
	b.WriteString(field("seo_title", meta.Title))
	b.WriteString(field("short_name", site.ShortName))
	if _, ok := opts.Existing["subtitle"]; ok || meta.Subtitle != "" {
		b.WriteString(field("subtitle", meta.Subtitle))
	}
	b.WriteString(field("tags", meta.Tags))
	b.WriteString(field("theme-color", "0, 83, 191"))
	b.WriteString(field("title", meta.Title))
	b.WriteString(field("url", fullURL))
	b.WriteString(field("viewport", "width=device-width, initial-scale=1, shrink-to-fit=no"))
	b.WriteString("\n")

	b.WriteString("# RSS - The RSS feed front matter (YAML).\n")
	b.WriteString(field("atom_link", rssURL))
	b.WriteString(field("category", meta.Category))
	if v, ok := opts.Existing["docs"]; ok {
		b.WriteString("docs: " + v + "\n")
	} else {
		b.WriteString("docs: https://validator.w3.org/feed/docs/rss2.html\n")
	}
	b.WriteString(field("generator", "Static Site Generator (SSG) (version 0.0.26)"))
	b.WriteString(field("item_description", meta.Description))
	b.WriteString(field("item_guid", rssURL))
	b.WriteString(field("item_link", rssURL))
	b.WriteString(field("item_pub_date", rssDate))
	b.WriteString(field("item_title", meta.Title))
	b.WriteString(field("last_build_date", rssDate))
	b.WriteString(field("managing_editor", author))
	b.WriteString(field("pub_date", rssDate))
	b.WriteString(field("ttl", "60"))
	b.WriteString(field("type", "article"))
	b.WriteString(field("webmaster", site.Email))
	b.WriteString("\n")

	b.WriteString("# Apple - The Apple front matter (YAML).\n")
	b.WriteString(field("apple_mobile_web_app_orientations", "portrait"))
	b.WriteString(field("apple_touch_icon_sizes", "192x192"))
	b.WriteString(field("apple-mobile-web-app-capable", "yes"))
	b.WriteString(field("apple-mobile-web-app-status-bar-inset", "black"))
	b.WriteString(field("apple-mobile-web-app-status-bar-style", "black-translucent"))
	b.WriteString(field("apple-mobile-web-app-title", meta.Title))
	b.WriteString(field("apple-touch-fullscreen", "yes"))
	b.WriteString("\n")

	b.WriteString("# MS Application - The MS Application front matter (YAML).\n\n")
	b.WriteString(field("msapplication-navbutton-color", "0, 83, 191"))
	b.WriteString("\n")

	b.WriteString("# Twitter Card - The Twitter Card front matter (YAML).\n\n")
	b.WriteString(field("twitter_card", "summary_large_image"))
	b.WriteString(field("twitter_creator", site.TwitterHandle))
	b.WriteString(field("twitter_description", meta.Description))
	b.WriteString(field("twitter_image", logoURL))
	b.WriteString(field("twitter_image_alt", "Logo of "+site.Name))
	b.WriteString(field("twitter_site", site.TwitterHandle))
	b.WriteString(field("twitter_title", meta.Title))
	b.WriteString(field("twitter_url", fullURL))
	b.WriteString("\n")

	b.WriteString(field("excerpt", meta.Excerpt))
	b.WriteString("\n")

	b.WriteString("# Humans.txt - The Humans.txt front matter (YAML).\n")
	b.WriteString(field("author_website", site.BaseURL))
	b.WriteString(field("author_twitter", site.TwitterHandle))
	b.WriteString(field("author_location", site.Location))
	b.WriteString(field("thanks", "Thanks for reading!"))
	b.WriteString(field("site_last_updated", dateStr))
	b.WriteString(field("site_standards", "HTML5, CSS3, RSS, Atom, JSON, XML, YAML, Markdown, TOML"))
	b.WriteString(field("site_components", "Kaishi, Kaishi Builder, Kaishi CLI, Kaishi Templates, Kaishi Themes"))
	b.WriteString("\n")

	b.WriteString("---")
	return b.String()
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

// ProcessFile reads a Markdown file, generates adjacent frontmatter, and writes the body,
// adjacent frontmatter, and final combined files. The filename is the article's
// publishing identity: a YYYY-MM-DD prefix takes precedence over date and the
// remaining stem becomes the canonical slug, so regenerating never changes an
// article's permalink. Existing frontmatter — from the input itself or from the
// adjacent -frontmatter.yaml — is preserved field by field; only missing fields
// are generated from the body.
func ProcessFile(inputPath string, date time.Time) (bodyPath, frontmatterPath, finalPath string, err error) {
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	if strings.EqualFold(ext, ".yaml") || strings.EqualFold(ext, ".yml") {
		return "", "", "", fmt.Errorf("%s is a frontmatter file, not an article body", inputPath)
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		return "", "", "", err
	}

	fmBlock, body := Split(string(data))
	body = strings.TrimSpace(body)
	if body == "" {
		return "", "", "", fmt.Errorf("no article body found in %s", inputPath)
	}

	dir := filepath.Dir(inputPath)
	stem := strings.TrimSuffix(base, ext)

	// Clean suffixes if processing an already generated -body or -final file.
	stemClean := stem
	stemClean = strings.TrimSuffix(stemClean, "-body")
	stemClean = strings.TrimSuffix(stemClean, "-final")
	stemClean = strings.TrimSuffix(stemClean, "-frontmatter")

	// Day-folder layout: when the input sits in a source/ or final/
	// directory, the three outputs route to the sibling source/, yaml/
	// and final/ directories instead of piling up beside the input.
	outSource, outYAML, outFinal := dir, dir, dir
	if name := filepath.Base(dir); name == "source" || name == "final" {
		parent := filepath.Dir(dir)
		outSource = filepath.Join(parent, "source")
		outYAML = filepath.Join(parent, "yaml")
		outFinal = filepath.Join(parent, "final")
	}
	bodyPath = filepath.Join(outSource, stemClean+"-body.md")
	frontmatterPath = filepath.Join(outYAML, stemClean+"-frontmatter.yaml")
	finalPath = filepath.Join(outFinal, stemClean+"-final.md")

	slugSource := stemClean
	if m := datePrefixPat.FindStringSubmatch(stemClean); m != nil {
		if parsed, perr := time.Parse("2006-01-02", m[1]); perr == nil {
			date = parsed
		}
		slugSource = strings.TrimPrefix(stemClean, m[0])
	}
	slug := ""
	if slugSource != "" {
		slug = Slugify(slugSource)
	}
	if date.IsZero() {
		date = time.Now()
	}

	// Curated metadata survives regeneration: the adjacent frontmatter file
	// seeds the existing fields, and the input's own block overrides it.
	existing := make(map[string]string)
	if adjacent, rerr := os.ReadFile(frontmatterPath); rerr == nil {
		for k, v := range parseFields(string(adjacent)) {
			existing[k] = v
		}
	}
	for k, v := range parseFields(fmBlock) {
		existing[k] = v
	}

	fmYAML := GenerateWithOptions(body, Options{Date: date, Slug: slug, Existing: existing})
	finalMD := Combine(fmYAML, body)

	for _, p := range []string{bodyPath, frontmatterPath, finalPath} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", "", "", err
		}
	}
	if err := os.WriteFile(bodyPath, []byte(body+"\n"), 0o644); err != nil {
		return "", "", "", err
	}
	if err := os.WriteFile(frontmatterPath, []byte(fmYAML+"\n"), 0o644); err != nil {
		return "", "", "", err
	}
	if err := os.WriteFile(finalPath, []byte(finalMD+"\n"), 0o644); err != nil {
		return "", "", "", err
	}

	return bodyPath, frontmatterPath, finalPath, nil
}

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

func quoteYAML(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", " ")
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
