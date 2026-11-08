// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package provenance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// --- Verification -----------------------------------------------------------
//
// A signed C2PA manifest proves who made a file and that it has not changed
// since. draft signs nothing, but the grounding assertion still binds the
// article to the digest of the exact body it was written from and to the
// ledger it was verified against. CheckArticle recomputes those digests and
// reports whether they still hold — the offline half of provenance, usable
// before any publisher signs anything.

// manifestFile is the read-back shape of a manifest written by NewManifest.
type manifestFile struct {
	ClaimGeneratorInfo []Generator `json:"claim_generator_info"`
	Title              string      `json:"title"`
	Assertions         []struct {
		Label string          `json:"label"`
		Data  json.RawMessage `json:"data"`
	} `json:"assertions"`
}

// ParseManifest reads a manifest written by NewManifest and returns its
// grounding assertion. It errors if the bytes are not such a manifest.
func ParseManifest(b []byte) (Grounding, Generator, error) {
	var mf manifestFile
	if err := json.Unmarshal(b, &mf); err != nil {
		return Grounding{}, Generator{}, fmt.Errorf("not a draft manifest: %w", err)
	}
	var gen Generator
	if len(mf.ClaimGeneratorInfo) > 0 {
		gen = mf.ClaimGeneratorInfo[0]
	}
	for _, a := range mf.Assertions {
		if a.Label == GroundingLabel {
			var g Grounding
			if err := json.Unmarshal(a.Data, &g); err != nil {
				return Grounding{}, gen, fmt.Errorf("malformed grounding assertion: %w", err)
			}
			return g, gen, nil
		}
	}
	return Grounding{}, gen, fmt.Errorf("no %s assertion in the manifest", GroundingLabel)
}

// SourceCheck is the outcome of checking one recorded source against the file
// on disk, when it can be found.
type SourceCheck struct {
	Path    string
	Found   bool
	Matches bool
}

// CheckReport is the outcome of CheckArticle.
type CheckReport struct {
	Generator     Generator
	Grounding     Grounding
	BodySHA256    string
	DigestMatches bool
	// LedgerMatches is set only when a ledger digest was supplied to check.
	LedgerChecked bool
	LedgerMatches bool
	Sources       []SourceCheck
	Problems      []string
	// SourceType is the digitalSourceType the manifest's created action
	// records: trainedAlgorithmicMedia for everything draft writes.
	SourceType string
	// Review is the recorded human review, if any, and ReviewMatches whether
	// it covers the text being checked.
	Review        *Review
	ReviewMatches bool
}

// OK reports whether the article still matches everything the manifest binds:
// the body digest, or a review of exactly this text, the ledger digest if one
// was checked, and every source that could be found on disk. A reviewed edit
// is accounted for: someone took responsibility for the text as it stands.
func (r CheckReport) OK() bool {
	if !r.DigestMatches && !r.ReviewMatches || len(r.Problems) > 0 {
		return false
	}
	if r.LedgerChecked && !r.LedgerMatches {
		return false
	}
	for _, s := range r.Sources {
		if s.Found && !s.Matches {
			return false
		}
	}
	return true
}

// CheckArticle recomputes the body's digest and, when the files are reachable,
// the ledger and source digests, and reports whether they match what the
// manifest recorded. It never fetches anything: a source that is not on disk
// is reported as not found, not as a failure, because provenance travels
// without the inputs.
func CheckArticle(body string, manifest []byte, ledger []byte, resolveSource func(name string) (string, bool)) CheckReport {
	var r CheckReport
	g, gen, err := ParseManifest(manifest)
	if err != nil {
		r.Problems = append(r.Problems, err.Error())
		return r
	}
	r.Generator, r.Grounding = gen, g
	if err := r.readDisclosure(manifest); err != nil {
		r.Problems = append(r.Problems, err.Error())
		return r
	}

	sum := sha256.Sum256([]byte(body))
	r.BodySHA256 = hex.EncodeToString(sum[:])
	r.DigestMatches = g.ArticleSHA256 != "" && r.BodySHA256 == g.ArticleSHA256
	r.ReviewMatches = r.Review.covers(r.BodySHA256)
	if g.ArticleSHA256 == "" {
		r.Problems = append(r.Problems, "the manifest records no article digest")
	}

	if ledger != nil && g.LedgerSHA256 != "" {
		r.LedgerChecked = true
		lsum := sha256.Sum256(ledger)
		r.LedgerMatches = hex.EncodeToString(lsum[:]) == g.LedgerSHA256
	}

	for _, s := range g.Sources {
		check := SourceCheck{Path: s.Path}
		if resolveSource != nil {
			if got, ok := resolveSource(s.Path); ok {
				check.Found = true
				check.Matches = got == s.SHA256
			}
		}
		r.Sources = append(r.Sources, check)
	}
	return r
}
