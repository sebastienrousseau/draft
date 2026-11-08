// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package frontmatter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sebastienrousseau/draft/internal/atomicfile"
)

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
	// Site is the publisher identity; nil means DefaultSite, which is empty.
	Site *Site
	// Engine, Model and Version record what wrote the article: the backend
	// that produced the body, the model it used, and the draft release that
	// ran. They are emitted only when set, so a set regenerated from its
	// files keeps whatever provenance it already carries and gains none it
	// cannot prove.
	Engine  string
	Model   string
	Version string
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
	// optional is field for a key that exists only when there is a value:
	// an existing one is kept, a generated one is written, and neither
	// yields an empty line that would look like a fact.
	optional := func(key, generated string) string {
		if _, ok := opts.Existing[key]; ok {
			return field(key, generated)
		}
		if strings.TrimSpace(generated) == "" {
			return ""
		}
		return field(key, generated)
	}

	dateStr := date.Format("2006-01-02")
	dateDisplay := date.Format("January 2, 2006")
	rssDate := date.Format("Mon, 02 Jan 2006 15:04:05 +0000")
	dateSlug := dateStr + "-" + meta.Slug
	sf := site.fields(dateSlug, meta.Slug, date.Year())
	bannerAlt := fmt.Sprintf("An illustration representing %s.", strings.ToLower(meta.Title))

	var b strings.Builder
	b.WriteString("---\n\n")
	b.WriteString("# Front Matter (YAML)\n\n")
	b.WriteString(optional("author", sf.author))
	b.WriteString(optional("banner_alt", ifSet(bannerAlt, sf.banner)))
	b.WriteString(optional("banner_height", ifSet("1280", sf.banner)))
	b.WriteString(optional("banner_width", ifSet("1920", sf.banner)))
	b.WriteString(optional("banner", sf.banner))
	b.WriteString(optional("cdn", site.CDN))
	b.WriteString(field("charset", "UTF-8"))
	b.WriteString(optional("cname", sf.cname))
	b.WriteString(optional("copyright", sf.copyright))
	b.WriteString(field("date", dateDisplay))
	b.WriteString(field("description", meta.Description))
	b.WriteString(field("format-detection", "telephone=no"))
	b.WriteString(field("hreflang", "en"))
	b.WriteString(optional("icon", sf.logo))
	b.WriteString(optional("id", sf.url))
	b.WriteString(optional("image_alt", sf.imageAlt))
	b.WriteString(optional("image_height", ifSet("162", sf.image)))
	b.WriteString(optional("image_width", ifSet("162", sf.image)))
	b.WriteString(optional("image", sf.image))
	b.WriteString(field("keywords", meta.Keywords))
	b.WriteString(field("language", "en-GB"))
	b.WriteString(field("last_reviewed", dateStr))
	b.WriteString(field("layout", "report"))
	b.WriteString(field("locale", "en_GB"))
	b.WriteString(optional("logo_alt", sf.logoAlt))
	b.WriteString(optional("logo_height", ifSet("44", sf.logo)))
	b.WriteString(optional("logo_width", ifSet("44", sf.logo)))
	b.WriteString(optional("logo", sf.logo))
	b.WriteString(field("menu", ""))
	b.WriteString(optional("measurementID", site.MeasurementID))
	b.WriteString(optional("name", site.Name))
	b.WriteString(optional("permalink", sf.url))
	b.WriteString(field("rating", "general"))
	b.WriteString(field("referrer", "no-referrer"))
	b.WriteString(field("robots", "index, follow"))
	b.WriteString(field("schema", "FAQPage, Article"))
	b.WriteString(field("seo_title", meta.Title))
	b.WriteString(optional("short_name", site.ShortName))
	if _, ok := opts.Existing["subtitle"]; ok || meta.Subtitle != "" {
		b.WriteString(field("subtitle", meta.Subtitle))
	}
	b.WriteString(field("tags", meta.Tags))
	b.WriteString(field("theme-color", "0, 83, 191"))
	b.WriteString(field("title", meta.Title))
	b.WriteString(optional("url", sf.url))
	b.WriteString(field("viewport", "width=device-width, initial-scale=1, shrink-to-fit=no"))
	b.WriteString("\n")

	b.WriteString("# RSS - The RSS feed front matter (YAML).\n")
	b.WriteString(optional("atom_link", sf.rss))
	b.WriteString(field("category", meta.Category))
	if v, ok := opts.Existing["docs"]; ok {
		b.WriteString("docs: " + v + "\n")
	} else {
		b.WriteString("docs: https://validator.w3.org/feed/docs/rss2.html\n")
	}
	b.WriteString(field("generator", "Static Site Generator (SSG) (version 0.0.26)"))
	b.WriteString(optional("draft_engine", opts.Engine))
	b.WriteString(optional("draft_model", opts.Model))
	b.WriteString(optional("draft_version", opts.Version))
	b.WriteString(field("item_description", meta.Description))
	b.WriteString(optional("item_guid", sf.rss))
	b.WriteString(optional("item_link", sf.rss))
	b.WriteString(field("item_pub_date", rssDate))
	b.WriteString(field("item_title", meta.Title))
	b.WriteString(field("last_build_date", rssDate))
	b.WriteString(optional("managing_editor", sf.author))
	b.WriteString(field("pub_date", rssDate))
	b.WriteString(field("ttl", "60"))
	b.WriteString(field("type", "article"))
	b.WriteString(optional("webmaster", site.Email))
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
	b.WriteString(optional("twitter_creator", site.TwitterHandle))
	b.WriteString(field("twitter_description", meta.Description))
	b.WriteString(optional("twitter_image", sf.logo))
	b.WriteString(optional("twitter_image_alt", sf.twitterImageAlt))
	b.WriteString(optional("twitter_site", site.TwitterHandle))
	b.WriteString(field("twitter_title", meta.Title))
	b.WriteString(optional("twitter_url", sf.url))
	b.WriteString("\n")

	b.WriteString(field("excerpt", meta.Excerpt))
	b.WriteString("\n")

	b.WriteString("# Humans.txt - The Humans.txt front matter (YAML).\n")
	b.WriteString(optional("author_website", site.BaseURL))
	b.WriteString(optional("author_twitter", site.TwitterHandle))
	b.WriteString(optional("author_location", site.Location))
	b.WriteString(field("thanks", "Thanks for reading!"))
	b.WriteString(field("site_last_updated", dateStr))
	b.WriteString(field("site_standards", "HTML5, CSS3, RSS, Atom, JSON, XML, YAML, Markdown, TOML"))
	b.WriteString(field("site_components", "Kaishi, Kaishi Builder, Kaishi CLI, Kaishi Templates, Kaishi Themes"))
	b.WriteString("\n")

	b.WriteString("---")
	return b.String()
}

// ProcessFile reads a Markdown file, generates adjacent frontmatter, and writes the body,
// adjacent frontmatter, and final combined files. The filename is the article's
// publishing identity: a YYYY-MM-DD prefix takes precedence over date and the
// remaining stem becomes the canonical slug, so regenerating never changes an
// article's permalink. Existing frontmatter — from the input itself or from the
// adjacent -frontmatter.yaml — is preserved field by field; only missing fields
// are generated from the body.
//
// The publisher identity comes from SiteFromEnv; ProcessFileWithSite takes
// one explicitly.
func ProcessFile(inputPath string, date time.Time) (bodyPath, frontmatterPath, finalPath string, err error) {
	return ProcessFileWithSite(inputPath, date, SiteFromEnv())
}

// ProcessFileWithSite is ProcessFile with the publisher identity given.
func ProcessFileWithSite(inputPath string, date time.Time, site Site) (bodyPath, frontmatterPath, finalPath string, err error) {
	ext := filepath.Ext(inputPath)
	if strings.EqualFold(ext, ".yaml") || strings.EqualFold(ext, ".yml") {
		return "", "", "", fmt.Errorf("%s is a frontmatter file, not an article body", inputPath)
	}
	data, err := readCapped(inputPath, MaxArticleBytes)
	if err != nil {
		return "", "", "", err
	}
	fmBlock, body := Split(string(data))
	body = strings.TrimSpace(body)
	if body == "" {
		return "", "", "", fmt.Errorf("no article body found in %s", inputPath)
	}

	stem, bodyPath, frontmatterPath, finalPath := setPaths(inputPath)
	slug, date := identityFromStem(stem, date)
	existing := existingFields(frontmatterPath, fmBlock)
	fmYAML := GenerateWithOptions(body, Options{Date: date, Slug: slug, Existing: existing, Site: &site})

	// Write the trio as a unit. Three sequential os.WriteFile calls truncate
	// the user's existing files one at a time, so a failure after the first
	// left the set disagreeing with itself — breaking the guarantee that the
	// body, frontmatter and final document are always in step.
	set := map[string][]byte{
		bodyPath:        []byte(body + "\n"),
		frontmatterPath: []byte(fmYAML + "\n"),
		finalPath:       []byte(Combine(fmYAML, body) + "\n"),
	}
	for p := range set {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", "", "", err
		}
	}
	if err := atomicfile.WriteSet(set, 0o644); err != nil {
		return "", "", "", err
	}
	return bodyPath, frontmatterPath, finalPath, nil
}

// setPaths returns the article's stem, stripped of any -body, -final or
// -frontmatter suffix, and the paths of the three files in its set.
//
// Day-folder layout: when the input sits in a source/ or final/ directory,
// the three outputs route to the sibling source/, yaml/ and final/
// directories instead of piling up beside the input.
func setPaths(inputPath string) (stem, bodyPath, frontmatterPath, finalPath string) {
	base := filepath.Base(inputPath)
	stem = strings.TrimSuffix(base, filepath.Ext(base))
	for _, suffix := range []string{"-body", "-final", "-frontmatter"} {
		stem = strings.TrimSuffix(stem, suffix)
	}
	dir := filepath.Dir(inputPath)
	outSource, outYAML, outFinal := dir, dir, dir
	if name := filepath.Base(dir); name == "source" || name == "final" {
		parent := filepath.Dir(dir)
		outSource = filepath.Join(parent, "source")
		outYAML = filepath.Join(parent, "yaml")
		outFinal = filepath.Join(parent, "final")
	}
	return stem,
		filepath.Join(outSource, stem+"-body.md"),
		filepath.Join(outYAML, stem+"-frontmatter.yaml"),
		filepath.Join(outFinal, stem+"-final.md")
}

// identityFromStem reads the publishing date and slug from a set's stem. A
// YYYY-MM-DD prefix wins over date, and a zero date means now.
func identityFromStem(stem string, date time.Time) (string, time.Time) {
	slugSource := stem
	if m := datePrefixPat.FindStringSubmatch(stem); m != nil {
		if parsed, err := time.Parse("2006-01-02", m[1]); err == nil {
			date = parsed
		}
		slugSource = strings.TrimPrefix(stem, m[0])
	}
	if date.IsZero() {
		date = time.Now()
	}
	if slugSource == "" {
		return "", date
	}
	return Slugify(slugSource), date
}

// existingFields gathers the curated metadata that survives regeneration:
// the adjacent frontmatter file seeds it, and the input's own block
// overrides that.
func existingFields(frontmatterPath, fmBlock string) map[string]string {
	existing := make(map[string]string)
	if adjacent, err := os.ReadFile(frontmatterPath); err == nil {
		for k, v := range parseFields(string(adjacent)) {
			existing[k] = v
		}
	}
	for k, v := range parseFields(fmBlock) {
		existing[k] = v
	}
	return existing
}
