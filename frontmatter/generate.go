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
	// Site is the publisher identity; nil means DefaultSite.
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
	b.WriteString(optional("draft_engine", opts.Engine))
	b.WriteString(optional("draft_model", opts.Model))
	b.WriteString(optional("draft_version", opts.Version))
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

	data, err := readCapped(inputPath, MaxArticleBytes)
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

	site := SiteFromEnv()
	fmYAML := GenerateWithOptions(body, Options{Date: date, Slug: slug, Existing: existing, Site: &site})
	finalMD := Combine(fmYAML, body)

	for _, p := range []string{bodyPath, frontmatterPath, finalPath} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", "", "", err
		}
	}
	// Write the trio as a unit. Three sequential os.WriteFile calls truncate
	// the user's existing files one at a time, so a failure after the first
	// left the set disagreeing with itself — breaking the guarantee that the
	// body, frontmatter and final document are always in step.
	if err := atomicfile.WriteSet(map[string][]byte{
		bodyPath:        []byte(body + "\n"),
		frontmatterPath: []byte(fmYAML + "\n"),
		finalPath:       []byte(finalMD + "\n"),
	}, 0o644); err != nil {
		return "", "", "", err
	}

	return bodyPath, frontmatterPath, finalPath, nil
}
