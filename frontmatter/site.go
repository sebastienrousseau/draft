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

// SiteFromEnv returns DefaultSite with any DRAFT_SITE_* environment overrides
// applied: BASE_URL, CDN, NAME, SHORT_NAME, EMAIL, TWITTER, LOCATION,
// MEASUREMENT_ID, and COPYRIGHT_FROM. Unset or malformed values keep the
// default, so a partial override is always safe.
func SiteFromEnv() Site {
	s := DefaultSite
	set := func(dst *string, key string) {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			*dst = v
		}
	}
	set(&s.BaseURL, "DRAFT_SITE_BASE_URL")
	set(&s.CDN, "DRAFT_SITE_CDN")
	set(&s.Name, "DRAFT_SITE_NAME")
	set(&s.ShortName, "DRAFT_SITE_SHORT_NAME")
	set(&s.Email, "DRAFT_SITE_EMAIL")
	set(&s.TwitterHandle, "DRAFT_SITE_TWITTER")
	set(&s.Location, "DRAFT_SITE_LOCATION")
	set(&s.MeasurementID, "DRAFT_SITE_MEASUREMENT_ID")
	if v := os.Getenv("DRAFT_SITE_COPYRIGHT_FROM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			s.CopyrightFrom = n
		}
	}
	return s
}
