// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package frontmatter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

const sampleArticle = `# From Evidence to Truth: Why Certified Blockchains Will Define Banking Trust

**ISO/IEC TC 307 guidance is not assurance, and scoring ledger governance against a 5-level maturity model turns engineering metrics into board-auditable truth.**

<!-- lead-start -->
<aside class="post-lead" aria-label="Article summary">
<p class="post-lead-tldr"><strong>TL;DR.</strong> Banks certify the cloud and AI, but not the ledger that decides what is true. A 5-level Certified Blockchain Index closes the fiduciary gap.</p>
<p class="post-lead-heading"><strong>Key takeaways</strong></p>
<ul class="post-lead-takeaways">
  <li><strong>Fiduciary gap.</strong> Current assurance models miss distributed ledgers.</li>
  <li><strong>DORA compliance.</strong> Boards face non-delegable liability under DORA Article 5.</li>
</ul>
</aside>
<!-- lead-end -->

> **Executive Summary**
>
> - Banks certify the cloud, entity, and AI, but not the ledger.
> - DORA Article 5 enforces board personal liability.

## The Fiduciary Frictional Gap in Digital Banking

In classical banking, trust is relational, institutional, and retrospective. It depends on independent auditors reviewing financial state.
`

func TestExtractMetadata(t *testing.T) {
	meta := ExtractMetadata(sampleArticle)
	if meta.Title != "From Evidence to Truth: Why Certified Blockchains Will Define Banking Trust" {
		t.Errorf("unexpected title: %q", meta.Title)
	}
	if !strings.Contains(meta.Subtitle, "ISO/IEC TC 307 guidance") {
		t.Errorf("unexpected subtitle: %q", meta.Subtitle)
	}
	if !strings.Contains(meta.Description, "Banks certify the cloud") {
		t.Errorf("unexpected description: %q", meta.Description)
	}
	if meta.Category != "Finance" {
		t.Errorf("expected Finance category, got %q", meta.Category)
	}
}

func TestGenerateYAML(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	yaml := Generate(sampleArticle, now)

	if !strings.HasPrefix(yaml, "---\n\n# Front Matter (YAML)") {
		t.Errorf("unexpected frontmatter prefix: %s", yaml[:40])
	}
	if !strings.HasSuffix(yaml, "---") {
		t.Errorf("unexpected frontmatter suffix")
	}
	if !strings.Contains(yaml, `title: "From Evidence to Truth: Why Certified Blockchains Will Define Banking Trust"`) {
		t.Errorf("missing title in YAML: %s", yaml)
	}
	if !strings.Contains(yaml, `date: "July 26, 2026"`) {
		t.Errorf("missing date display in YAML: %s", yaml)
	}
}

func TestCombineAndSplit(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	yaml := Generate(sampleArticle, now)
	combined := Combine(yaml, sampleArticle)

	splitFM, splitBody := Split(combined)
	if !strings.Contains(splitFM, "author:") {
		t.Errorf("split frontmatter missing author")
	}
	if !strings.HasPrefix(splitBody, "# From Evidence to Truth") {
		t.Errorf("split body missing H1 title: %s", splitBody[:50])
	}
}

func TestExtractMetadataDeckSubtitle(t *testing.T) {
	art := "# A Title\n\n<p class=\"deck\">A standfirst drawn from the deck paragraph.</p>\n\nProse follows here.\n"
	meta := ExtractMetadata(art)
	if meta.Subtitle != "A standfirst drawn from the deck paragraph." {
		t.Errorf("subtitle should come from the deck paragraph, got %q", meta.Subtitle)
	}
	// A bold line still works when there is no deck.
	if got := ExtractMetadata(sampleArticle).Subtitle; !strings.Contains(got, "ISO/IEC TC 307") {
		t.Errorf("bold-line subtitle regressed: %q", got)
	}
}

func TestExtractKeywordsQuality(t *testing.T) {
	art := "# Quantum Error Correction\n\n" +
		"## Why the overhead matters, and what it costs\n\n" +
		strings.Repeat("Their approach would improve which decoder should process syndrome measurements around lattice surgery. ", 10)
	meta := ExtractMetadata(art)
	for _, kw := range strings.Split(meta.Keywords, ", ") {
		switch kw {
		case "their", "would", "which", "should", "around", "improve", "process":
			t.Errorf("filler word leaked into keywords: %q (keywords: %s)", kw, meta.Keywords)
		}
	}
	if strings.Contains(meta.Keywords, "Why the overhead matters") {
		t.Errorf("H2 sentence fragment leaked into keywords: %s", meta.Keywords)
	}
	if !strings.Contains(meta.Keywords, "quantum") || !strings.Contains(meta.Keywords, "decoder") {
		t.Errorf("topical terms missing from keywords: %s", meta.Keywords)
	}
}

func TestInferCategoryScoring(t *testing.T) {
	// A single passing finance mention must not outweigh a dominant AI theme.
	art := "# Agent Autonomy in Production\n\n" +
		"One bank ran a pilot, but the story is agents: agents adapt, models reason, " +
		"AI plans ahead, agents recover from errors, and models improve with evaluation. " +
		"Agent frameworks and model routing decide everything."
	if got := ExtractMetadata(art).Category; got != "AI" {
		t.Errorf("category = %q, want AI (dominant theme should beat a passing mention)", got)
	}
}

func TestGenerateCopyrightYear(t *testing.T) {
	yaml := Generate("# T\n\nSome body.", time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC))
	if !strings.Contains(yaml, `copyright: "© Copyright 2025 - 2027 - Sebastien Rousseau. All rights reserved."`) {
		t.Error("copyright end year should follow the article date")
	}
}

func TestGenerateWithCustomSite(t *testing.T) {
	site := Site{
		BaseURL:       "https://example.org",
		CDN:           "https://cdn.example.org",
		Name:          "Ada Example",
		ShortName:     "adaexample",
		Email:         "ada@example.org",
		TwitterHandle: "@ada",
		Location:      "Paris, FR",
		MeasurementID: "G-TEST123",
		CopyrightFrom: 2020,
	}
	yaml := GenerateWithOptions("# T\n\nSome body.", Options{
		Date: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Site: &site,
	})
	for _, want := range []string{
		`url: "https://example.org/2026-01-15-t"`,
		`name: "Ada Example"`,
		`twitter_site: "@ada"`,
		`measurementID: "G-TEST123"`,
		`author_location: "Paris, FR"`,
	} {
		if !strings.Contains(yaml, want) {
			t.Errorf("custom site value missing: %s", want)
		}
	}
	if strings.Contains(yaml, "sebastienrousseau") {
		t.Error("default site identity leaked into custom-site output")
	}
}

func TestCombineClosesUnterminatedFrontmatter(t *testing.T) {
	// A frontmatter block whose last line merely ENDS in "---" is not closed;
	// Combine must still append the closing delimiter line.
	fm := "---\nnote: value ending in ---"
	combined := Combine(fm, "# T\n\nBody prose.")
	_, body := Split(combined)
	if !strings.HasPrefix(body, "# T") {
		t.Errorf("unterminated frontmatter was not closed; split body = %q", body)
	}
}

func TestDescriptionTruncationKeepsValidUTF8(t *testing.T) {
	// A long unbroken multi-byte run must not be cut mid-rune.
	art := "# Titre\n\n**" + strings.Repeat("é", 150) + "**\n\nProse.\n"
	meta := ExtractMetadata(art)
	if !utf8.ValidString(meta.Description) {
		t.Errorf("description is not valid UTF-8: %q", meta.Description)
	}
	if !utf8.ValidString(meta.Excerpt) {
		t.Errorf("excerpt is not valid UTF-8: %q", meta.Excerpt)
	}
}

func TestCleanTextKeepsHashAndGreaterThan(t *testing.T) {
	meta := ExtractMetadata("# C# Compilers: Why 100% > 50% Matters\n\nProse.\n")
	if meta.Title != "C# Compilers: Why 100% > 50% Matters" {
		t.Errorf("title mangled: %q", meta.Title)
	}
}

func TestExtractMetadataFallbacks(t *testing.T) {
	// Executive summary drives the description when there is no TL;DR.
	exec := "# Title A\n\n> **Executive Summary**\n>\n> - First point here.\n> - Second point here.\n\nProse.\n"
	if got := ExtractMetadata(exec).Description; !strings.Contains(got, "First point here") {
		t.Errorf("description should come from the executive summary, got %q", got)
	}

	// Subtitle stands in when there is no TL;DR or executive summary.
	bold := "# Title B\n\n**A bold thesis line.**\n\nProse.\n"
	if got := ExtractMetadata(bold).Description; got != "A bold thesis line." {
		t.Errorf("description should fall back to the subtitle, got %q", got)
	}

	// The title is the last resort.
	if got := ExtractMetadata("# Only Title\n\nplain prose here.").Description; got != "Only Title" {
		t.Errorf("description should fall back to the title, got %q", got)
	}

	// Author lines are not subtitles.
	author := "# Title C\n\n**Author: Someone Else**\n\nProse.\n"
	if got := ExtractMetadata(author).Subtitle; got != "" {
		t.Errorf("author line should not become the subtitle, got %q", got)
	}

	// A missing H1 yields the placeholder title.
	if got := ExtractMetadata("just prose, no heading at all").Title; got != "Untitled Article" {
		t.Errorf("unexpected placeholder title: %q", got)
	}

	// Long descriptions truncate at a word with an ellipsis; long subtitles
	// cap the excerpt too.
	long := strings.TrimSpace(strings.Repeat("wordy ", 60))
	tldr := "# Title D\n\n**" + long + " and then even more subtitle text here**\n\n" +
		`<p class="post-lead-tldr"><strong>TL;DR.</strong> ` + long + `</p>` + "\n\nProse.\n"
	m := ExtractMetadata(tldr)
	if len(m.Description) > 200 || !strings.HasSuffix(m.Description, "...") {
		t.Errorf("description not truncated cleanly: %d bytes, %q", len(m.Description), m.Description)
	}
	if len(m.Excerpt) > 220 {
		t.Errorf("excerpt not truncated: %d bytes", len(m.Excerpt))
	}
}

func TestCombineEdges(t *testing.T) {
	// Empty frontmatter returns the body alone.
	if got := Combine("", "# B\n\nBody."); got != "# B\n\nBody." {
		t.Errorf("empty frontmatter should return the body, got %q", got)
	}
	// A bare field block gets wrapped in delimiters.
	fmPart, body := Split(Combine(`key: "v"`, "# B\n\nBody."))
	if !strings.Contains(fmPart, "key:") {
		t.Errorf("bare block not wrapped as frontmatter: %q", fmPart)
	}
	if !strings.HasPrefix(body, "# B") {
		t.Errorf("body lost in wrapping: %q", body)
	}
}

func TestSlugifyEdges(t *testing.T) {
	if got := Slugify("!!! ***"); got != "draft-article" {
		t.Errorf("no-letter input should use the placeholder slug, got %q", got)
	}
	long := strings.Repeat("word-", 30)
	got := Slugify(long)
	if len(got) > 90 || strings.HasSuffix(got, "-") {
		t.Errorf("long slug not truncated cleanly: %d chars, %q", len(got), got)
	}
}

func TestProcessFileErrors(t *testing.T) {
	if _, _, _, err := ProcessFile("/no/such/dir/article.md", time.Time{}); err == nil {
		t.Error("missing input should error")
	}

	// No date prefix: the whole stem is the slug and a zero date means now.
	dir := t.TempDir()
	f := filepath.Join(dir, "meeting-notes.md")
	if err := os.WriteFile(f, []byte("# Some Note Title\n\nBody prose.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, fmPath, _, err := ProcessFile(f, time.Time{})
	if err != nil {
		t.Fatalf("ProcessFile failed: %v", err)
	}
	fmContent, err := os.ReadFile(fmPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fmContent), "/meeting-notes") {
		t.Error("slug should come from the dateless filename stem")
	}
}

func TestGenerateZeroDateUsesNow(t *testing.T) {
	yaml := Generate("# T\n\nBody.", time.Time{})
	if !strings.Contains(yaml, `last_reviewed: "`+time.Now().Format("2006")+"") {
		t.Error("zero date should fall back to the current time")
	}
}

func TestSiteFromEnv(t *testing.T) {
	t.Setenv("DRAFT_SITE_BASE_URL", "https://blog.example.org")
	t.Setenv("DRAFT_SITE_NAME", "Ada Example")
	t.Setenv("DRAFT_SITE_TWITTER", "@ada")
	t.Setenv("DRAFT_SITE_COPYRIGHT_FROM", "2024")

	s := SiteFromEnv()
	if s.BaseURL != "https://blog.example.org" || s.Name != "Ada Example" ||
		s.TwitterHandle != "@ada" || s.CopyrightFrom != 2024 {
		t.Errorf("env overrides not applied: %+v", s)
	}
	// Unset variables keep the defaults.
	if s.CDN != DefaultSite.CDN || s.Email != DefaultSite.Email {
		t.Errorf("defaults lost for unset variables: %+v", s)
	}
	// A malformed year keeps the default.
	t.Setenv("DRAFT_SITE_COPYRIGHT_FROM", "not-a-year")
	if got := SiteFromEnv().CopyrightFrom; got != DefaultSite.CopyrightFrom {
		t.Errorf("malformed year should keep default, got %d", got)
	}
}

func TestProcessFileHonoursSiteEnv(t *testing.T) {
	t.Setenv("DRAFT_SITE_BASE_URL", "https://blog.example.org")

	dir := t.TempDir()
	src := filepath.Join(dir, "2026-01-15-env-site.md")
	if err := os.WriteFile(src, []byte("# Env Site\n\nBody prose.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, fmPath, _, err := ProcessFile(src, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ProcessFile failed: %v", err)
	}
	fmContent, err := os.ReadFile(fmPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fmContent), `permalink: "https://blog.example.org/2026-01-15-env-site"`) {
		t.Error("ProcessFile should honour DRAFT_SITE_* environment overrides")
	}
}

func TestPartOfSet(t *testing.T) {
	cases := map[string]bool{
		"/x/2026-01-15-a-body.md":        true,
		"/x/2026-01-15-a-final.md":       true,
		"/x/day/source/2026-01-15-a.md":  true,
		"/x/day/final/2026-01-15-a.md":   true,
		"/x/2026-01-15-plain-article.md": false,
		"/x/day/notes/2026-01-15-a.md":   false,
	}
	for path, want := range cases {
		if got := PartOfSet(path); got != want {
			t.Errorf("PartOfSet(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestInferCategoryWordBoundaries(t *testing.T) {
	cases := []struct {
		name, article, want string
	}{
		// Words like "said", "against", "remain" contain "ai" but are not AI content.
		{"cooking", "# My Favourite Pasta Recipes\n\nI said to my friend that we should remain calm and maintain the sauce temperature.", "Technology"},
		{"gardening", "# A Certain Way to Grow Tomatoes\n\nAgainst all odds, the garden flourished again this year.", "Technology"},
		{"golang", "# Understanding Goroutines\n\nChannels provide synchronisation. Details available in the docs.", "Technology"},
		{"ai-word", "# Why AI Agents Need Guardrails\n\nEvery model deployment should gate agent autonomy.", "AI"},
		{"finance", sampleArticle, "Finance"},
		{"security", "# Hardening the Perimeter\n\nSecurity audits under DORA demand evidence.", "Security"},
	}
	for _, c := range cases {
		if got := ExtractMetadata(c.article).Category; got != c.want {
			t.Errorf("%s: category = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestProcessFileRejectsFrontmatterFile(t *testing.T) {
	dir := t.TempDir()
	srcFile := filepath.Join(dir, "2026-07-26-sample.md")
	if err := os.WriteFile(srcFile, []byte(sampleArticle), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	bodyPath, fmPath, _, err := ProcessFile(srcFile, now)
	if err != nil {
		t.Fatalf("ProcessFile failed: %v", err)
	}
	bodyBefore, err := os.ReadFile(bodyPath)
	if err != nil {
		t.Fatal(err)
	}

	// Feeding the generated frontmatter file back in must error, not
	// silently overwrite the body files with an empty article.
	if _, _, _, err := ProcessFile(fmPath, now); err == nil {
		t.Error("ProcessFile should reject a -frontmatter.yaml input")
	}
	bodyAfter, err := os.ReadFile(bodyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(bodyAfter) != string(bodyBefore) {
		t.Errorf("body file was clobbered:\nbefore: %q\nafter:  %q", bodyBefore, bodyAfter)
	}

	// Same protection for any input whose extracted body is empty.
	emptyFile := filepath.Join(dir, "2026-07-26-empty.md")
	if err := os.WriteFile(emptyFile, []byte("---\nkey: value\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ProcessFile(emptyFile, now); err == nil {
		t.Error("ProcessFile should reject an input with no article body")
	}
}

func TestProcessFilePreservesFilenameDate(t *testing.T) {
	dir := t.TempDir()
	srcFile := filepath.Join(dir, "2026-01-15-dated-article.md")
	if err := os.WriteFile(srcFile, []byte(sampleArticle), 0o644); err != nil {
		t.Fatal(err)
	}

	// Regenerating later must keep the article's original date, not re-date
	// the permalink to the regeneration day.
	later := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	_, fmPath, _, err := ProcessFile(srcFile, later)
	if err != nil {
		t.Fatalf("ProcessFile failed: %v", err)
	}
	fmContent, err := os.ReadFile(fmPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fmContent), `date: "January 15, 2026"`) {
		t.Error("frontmatter should keep the filename date, not the regeneration date")
	}
	if strings.Contains(string(fmContent), "2026-07-26-") {
		t.Error("permalink/id should not be re-dated to the regeneration day")
	}
}

func TestProcessFileUsesFilenameSlug(t *testing.T) {
	dir := t.TempDir()
	srcFile := filepath.Join(dir, "2026-01-15-my-canonical-slug.md")
	content := "# A Completely Different Headline Than the Filename\n\nSome body prose.\n"
	if err := os.WriteFile(srcFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	_, fmPath, _, err := ProcessFile(srcFile, now)
	if err != nil {
		t.Fatalf("ProcessFile failed: %v", err)
	}
	fmContent, err := os.ReadFile(fmPath)
	if err != nil {
		t.Fatal(err)
	}

	// The filename is the article's publishing identity: the permalink must
	// use its slug, not one re-derived from the title.
	want := `permalink: "https://sebastienrousseau.com/2026-01-15-my-canonical-slug"`
	if !strings.Contains(string(fmContent), want) {
		t.Errorf("permalink should use the filename slug; frontmatter:\n%s", fmContent)
	}
	if strings.Contains(string(fmContent), "a-completely-different-headline") {
		t.Error("title-derived slug leaked into the frontmatter")
	}
}

func TestProcessFilePreservesExistingFields(t *testing.T) {
	dir := t.TempDir()
	curatedFM := `---

# Front Matter (YAML)

banner_alt: "A hand-drawn ledger on parchment"
category: "Technology"
keywords: "curated, keywords, list"
pub_date: "Mon, 09 Nov 2026 07:07:07 +0000"
seo_title: "Certified Blockchains: The Board's New Audit"
subtitle: "A curated subtitle that must survive"
---`
	curatedLines := []string{
		`banner_alt: "A hand-drawn ledger on parchment"`,
		`category: "Technology"`,
		`keywords: "curated, keywords, list"`,
		`pub_date: "Mon, 09 Nov 2026 07:07:07 +0000"`,
		`seo_title: "Certified Blockchains: The Board's New Audit"`,
		`subtitle: "A curated subtitle that must survive"`,
	}

	finalFile := filepath.Join(dir, "2026-01-15-curated-final.md")
	if err := os.WriteFile(finalFile, []byte(Combine(curatedFM, sampleArticle)), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	bodyPath, fmPath, _, err := ProcessFile(finalFile, now)
	if err != nil {
		t.Fatalf("ProcessFile failed: %v", err)
	}
	fmContent, err := os.ReadFile(fmPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range curatedLines {
		if !strings.Contains(string(fmContent), line) {
			t.Errorf("curated field lost on regeneration: %s", line)
		}
	}
	// Fields absent from the curated block are still generated from the body.
	if !strings.Contains(string(fmContent), `title: "From Evidence to Truth`) {
		t.Error("missing fields should be generated from the body")
	}

	// The regeneration workflow: edit the body file, rerun on it, and the
	// curated fields must survive via the adjacent frontmatter file.
	body, err := os.ReadFile(bodyPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := string(body) + "\nA brand-new closing paragraph.\n"
	if err := os.WriteFile(bodyPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ProcessFile(bodyPath, now); err != nil {
		t.Fatalf("ProcessFile on edited body failed: %v", err)
	}
	fmContent, err = os.ReadFile(fmPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range curatedLines {
		if !strings.Contains(string(fmContent), line) {
			t.Errorf("curated field lost after body edit: %s", line)
		}
	}
}

func TestProcessFileDayFolderLayout(t *testing.T) {
	day := t.TempDir()
	srcDir := filepath.Join(day, "source")
	yamlDir := filepath.Join(day, "yaml")
	finalDir := filepath.Join(day, "final")
	for _, d := range []string{srcDir, yamlDir, finalDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	curated := "---\n\ncategory: \"Technology\"\nseo_title: \"A Curated SEO Title\"\n---\n"
	if err := os.WriteFile(filepath.Join(yamlDir, "2026-01-15-day-folder-frontmatter.yaml"), []byte(curated), 0o644); err != nil {
		t.Fatal(err)
	}
	bodyFile := filepath.Join(srcDir, "2026-01-15-day-folder-body.md")
	if err := os.WriteFile(bodyFile, []byte("# Day Folder Article\n\nBody prose.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	bodyPath, fmPath, finalPath, err := ProcessFile(bodyFile, now)
	if err != nil {
		t.Fatalf("ProcessFile failed: %v", err)
	}

	// Outputs must route to the sibling source/, yaml/ and final/ folders,
	// not pile up next to the input.
	if got, want := bodyPath, bodyFile; got != want {
		t.Errorf("body path = %s, want %s", got, want)
	}
	if got, want := fmPath, filepath.Join(yamlDir, "2026-01-15-day-folder-frontmatter.yaml"); got != want {
		t.Errorf("frontmatter path = %s, want %s", got, want)
	}
	if got, want := finalPath, filepath.Join(finalDir, "2026-01-15-day-folder-final.md"); got != want {
		t.Errorf("final path = %s, want %s", got, want)
	}

	// The curated fields in the sibling yaml folder must survive.
	fmContent, err := os.ReadFile(fmPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{`category: "Technology"`, `seo_title: "A Curated SEO Title"`} {
		if !strings.Contains(string(fmContent), line) {
			t.Errorf("curated field lost in day-folder layout: %s", line)
		}
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("source/ should hold only the body file, got %d entries", len(entries))
	}
}

func TestProcessFileIdempotent(t *testing.T) {
	dir := t.TempDir()
	srcFile := filepath.Join(dir, "2026-01-15-idempotent-check.md")
	if err := os.WriteFile(srcFile, []byte(sampleArticle), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	bodyPath, fmPath, finalPath, err := ProcessFile(srcFile, now)
	if err != nil {
		t.Fatalf("first ProcessFile failed: %v", err)
	}
	read := func(p string) string {
		t.Helper()
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	body1, fm1, final1 := read(bodyPath), read(fmPath), read(finalPath)

	// Reprocessing the final output must be a no-op.
	if _, _, _, err := ProcessFile(finalPath, now.AddDate(0, 1, 0)); err != nil {
		t.Fatalf("second ProcessFile failed: %v", err)
	}
	if got := read(bodyPath); got != body1 {
		t.Error("body file changed on reprocessing")
	}
	if got := read(fmPath); got != fm1 {
		t.Errorf("frontmatter changed on reprocessing:\nfirst:\n%s\nsecond:\n%s", fm1, got)
	}
	if got := read(finalPath); got != final1 {
		t.Error("final file changed on reprocessing")
	}
}

func TestProcessFile(t *testing.T) {
	dir := t.TempDir()
	srcFile := filepath.Join(dir, "2026-07-26-sample.md")
	if err := os.WriteFile(srcFile, []byte(sampleArticle), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	bodyPath, fmPath, finalPath, err := ProcessFile(srcFile, now)
	if err != nil {
		t.Fatalf("ProcessFile failed: %v", err)
	}

	if !strings.HasSuffix(bodyPath, "-body.md") {
		t.Errorf("unexpected body path: %s", bodyPath)
	}
	if !strings.HasSuffix(fmPath, "-frontmatter.yaml") {
		t.Errorf("unexpected fm path: %s", fmPath)
	}
	if !strings.HasSuffix(finalPath, "-final.md") {
		t.Errorf("unexpected final path: %s", finalPath)
	}

	bodyContent, _ := os.ReadFile(bodyPath)
	if strings.Contains(string(bodyContent), "---") {
		t.Errorf("body file should not contain frontmatter")
	}

	fmContent, _ := os.ReadFile(fmPath)
	if !strings.Contains(string(fmContent), "# Front Matter (YAML)") {
		t.Errorf("frontmatter file should contain YAML block")
	}

	finalContent, _ := os.ReadFile(finalPath)
	if !strings.Contains(string(finalContent), "---") || !strings.Contains(string(finalContent), "# From Evidence to Truth") {
		t.Errorf("final file should contain both frontmatter and body")
	}
}
