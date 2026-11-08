// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package frontmatter

import (
	"os"
	"strconv"
	"strings"
)

// Site holds the publisher identity stamped into generated frontmatter.
type Site struct {
	BaseURL       string // canonical site root, e.g. https://example.com
	CDN           string // asset host, e.g. https://cdn.example.com
	Name          string // display name
	ShortName     string // slug-like identity used in asset paths
	Email         string
	TwitterHandle string
	Location      string
	MeasurementID string
	CopyrightFrom int // first year of the copyright range
}

// DefaultSite is the publisher identity used when Options.Site is nil. It is
// empty: draft publishes as nobody until told who you are, and every field
// that would need an unset part of the identity is left out of the
// frontmatter rather than filled with someone else's.
var DefaultSite = Site{}

// siteKeys names each string field of Site once. The DRAFT_SITE_* variable
// and the draft.toml [site] key are both derived from the name, so the two
// can never disagree.
var siteKeys = []struct {
	name  string
	field func(*Site) *string
}{
	{"base-url", func(s *Site) *string { return &s.BaseURL }},
	{"cdn", func(s *Site) *string { return &s.CDN }},
	{"name", func(s *Site) *string { return &s.Name }},
	{"short-name", func(s *Site) *string { return &s.ShortName }},
	{"email", func(s *Site) *string { return &s.Email }},
	{"twitter", func(s *Site) *string { return &s.TwitterHandle }},
	{"location", func(s *Site) *string { return &s.Location }},
	{"measurement-id", func(s *Site) *string { return &s.MeasurementID }},
}

// copyrightFromKey is the one integer field, kept out of siteKeys.
const copyrightFromKey = "copyright-from"

// Override returns s with every field that lookup reports set replaced.
// lookup is asked for each key by its draft.toml name (base-url, cdn, name,
// short-name, email, twitter, location, measurement-id, copyright-from) and
// returns "" for unset. A blank or malformed value keeps the field as it
// was, so a partial override is always safe.
func (s Site) Override(lookup func(key string) string) Site {
	for _, k := range siteKeys {
		if v := strings.TrimSpace(lookup(k.name)); v != "" {
			*k.field(&s) = v
		}
	}
	if n, err := strconv.Atoi(strings.TrimSpace(lookup(copyrightFromKey))); err == nil {
		s.CopyrightFrom = n
	}
	return s
}

// WithEnv returns s overridden by the DRAFT_SITE_* environment variables:
// DRAFT_SITE_BASE_URL for base-url, and so on for every key Override reads.
func (s Site) WithEnv() Site {
	return s.Override(func(key string) string {
		return os.Getenv("DRAFT_SITE_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_")))
	})
}

// SiteFromEnv returns DefaultSite with any DRAFT_SITE_* environment overrides
// applied. It does not read draft.toml; config.Load resolves both layers.
func SiteFromEnv() Site { return DefaultSite.WithEnv() }

// Identified reports whether s names a publisher at all: a display name or a
// site to link to. Without either, generated frontmatter is anonymous.
func (s Site) Identified() bool { return s.Name != "" || s.BaseURL != "" }

// siteFields are the frontmatter values derived from a Site for one article.
// Each is empty when the Site lacks a part it needs, so an unset identity
// yields no field instead of a broken URL or an empty name.
type siteFields struct {
	url, rss, cname, author, copyright string
	banner, logo, logoAlt, image       string
	imageAlt, twitterImageAlt          string
}

// fields derives the per-article values from s.
func (s Site) fields(dateSlug, slug string, year int) siteFields {
	f := siteFields{
		url:    ifSet(s.BaseURL+"/"+dateSlug, s.BaseURL),
		cname:  strings.TrimPrefix(s.BaseURL, "https://"),
		author: s.author(),
		banner: ifSet(s.CDN+"/stocks/images/"+slug+".webp", s.CDN),
		logo:   ifSet(s.CDN+"/clients/"+s.ShortName+"/v1/logos/"+s.ShortName+".svg", s.CDN, s.ShortName),
		image:  ifSet(s.CDN+"/stocks/images/"+s.ShortName+".webp", s.CDN, s.ShortName),
	}
	f.rss = ifSet(f.url+"/rss.xml", f.url)
	f.logoAlt = ifSet("Logo for "+s.Name, f.logo, s.Name)
	f.twitterImageAlt = ifSet("Logo of "+s.Name, f.logo, s.Name)
	f.imageAlt = ifSet("Black and White Portrait of "+s.Name, f.image, s.Name)
	f.copyright = ifSet("© Copyright "+strconv.Itoa(year)+" - "+s.Name+". All rights reserved.", s.Name)
	if s.CopyrightFrom > 0 && s.CopyrightFrom < year {
		f.copyright = ifSet("© Copyright "+strconv.Itoa(s.CopyrightFrom)+" - "+strconv.Itoa(year)+" - "+s.Name+". All rights reserved.", s.Name)
	}
	return f
}

// author is the RSS-style "email (name)", or whichever half is set.
func (s Site) author() string {
	switch {
	case s.Email != "" && s.Name != "":
		return s.Email + " (" + s.Name + ")"
	case s.Email != "":
		return s.Email
	default:
		return s.Name
	}
}

// ifSet returns v when every one of parts is non-empty, and "" otherwise.
func ifSet(v string, parts ...string) string {
	for _, p := range parts {
		if p == "" {
			return ""
		}
	}
	return v
}
