// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sebastienrousseau/draft/frontmatter"
	"github.com/sebastienrousseau/draft/internal/brand"
	"github.com/sebastienrousseau/draft/internal/c2pa"
	"github.com/sebastienrousseau/draft/provenance"
)

// Seams over the c2pa package so signature reporting is testable without the
// c2patool binary. Production points them at the real functions.
var (
	c2paAvail  = c2pa.Available
	c2paVerify = c2pa.Verify
)

// setSuffixes are the filename endings that identify one file of a draft set,
// longest first so "-frontmatter.yaml" is matched before a bare stem guess.
var setSuffixes = []string{"-body.md", "-final.md", "-frontmatter.yaml", "-c2pa.json", "-attribution.json"}

// locateSet resolves any one file of a day-folder set to the body Markdown and
// the C2PA manifest beside it. The layout is <day>/{source,yaml,final,provenance}/<stem>-<kind>,
// so the stem and day root are recovered from the given path and the siblings
// are addressed by convention.
func locateSet(path string) (bodyPath, manifestPath string, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	base := filepath.Base(abs)
	stem := ""
	for _, suf := range setSuffixes {
		if strings.HasSuffix(base, suf) {
			stem = strings.TrimSuffix(base, suf)
			break
		}
	}
	if stem == "" {
		return "", "", fmt.Errorf("%s is not a draft set file (expected one ending in %s)", base, strings.Join(setSuffixes, ", "))
	}
	// The kind directory (source/yaml/final/provenance) is the file's parent;
	// the day root is its grandparent.
	dayRoot := filepath.Dir(filepath.Dir(abs))
	bodyPath = filepath.Join(dayRoot, "source", stem+"-body.md")
	manifestPath = filepath.Join(dayRoot, "provenance", stem+"-c2pa.json")
	return bodyPath, manifestPath, nil
}

// loadSet resolves a set file, reads its body and manifest (and a ledger if one
// is still present), and runs the digest checks. A load failure is reported to
// stderr and ok is false. It is shared by the human and JSON verify paths so
// both check exactly the same thing.
func loadSet(path string, stderr io.Writer) (report provenance.CheckReport, bodyPath, manifestPath string, ok bool) {
	bodyPath, manifestPath, err := locateSet(path)
	if err != nil {
		fmt.Fprintln(stderr, "draft:", err)
		return
	}
	body, err := readBodyOrFinal(bodyPath, path)
	if err != nil {
		fmt.Fprintln(stderr, "draft:", err)
		return
	}
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "draft: no manifest for this article (%s); it was written before provenance existed, or the file was moved out of its set\n", filepath.Base(manifestPath))
		return
	}
	// The ledger is normally cleaned up after a successful run, so its digest
	// is checked only when a --keep-artifacts ledger is still beside the set.
	var ledger []byte
	dayRoot := filepath.Dir(filepath.Dir(manifestPath))
	if matches, _ := filepath.Glob(filepath.Join(dayRoot, "*-verified-claim-ledger.md")); len(matches) > 0 {
		ledger, _ = os.ReadFile(matches[0])
	}
	report = provenance.CheckArticle(body+"\n", manifest, ledger, resolveSourceDigest)
	ok = true
	return
}

// runVerify checks an article against the provenance written beside it and
// prints a human report, returning 0 when the article still matches its
// manifest and 1 when it does not or a file is missing.
func runVerify(path string, stdout, stderr io.Writer) int {
	report, bodyPath, manifestPath, ok := loadSet(path, stderr)
	if !ok {
		return 1
	}
	printVerifyReport(stdout, path, manifestPath, report)

	// A manifest that could not be read is already reported by
	// printVerifyReport; there is nothing further to check.
	if len(report.Problems) > 0 {
		return 1
	}

	// When a signed credential sits beside the set, validate its signature too,
	// binding it to the body file the manifest was computed over.
	sigOK := true
	if sig := signatureFor(bodyPath, manifestPath); sig != nil {
		sigOK = printSignatureReport(stdout, sig)
	}

	fmt.Fprintln(stdout)
	if report.OK() && sigOK {
		fmt.Fprintln(stdout, "  "+brand.Help.Render("Verified. The article matches the provenance written beside it."))
		return 0
	}
	fmt.Fprintln(stdout, "  "+brand.Accent.Render("Not verified.")+" The article or a source no longer matches its manifest.")
	return 1
}

// runVerifyJSON checks a set and prints a portable verification record — the
// machine-readable form of --verify — to stdout, exiting 0 when the record's
// verdict is verified. It is the stable seam a program (CI, a hosted verifier)
// consumes: bring your own artifact, verify anywhere.
func runVerifyJSON(path string, stdout, stderr io.Writer) int {
	report, bodyPath, manifestPath, ok := loadSet(path, stderr)
	if !ok {
		return 1
	}
	var sig *provenance.SignatureState
	if len(report.Problems) == 0 {
		sig = signatureFor(bodyPath, manifestPath)
	}
	record := provenance.NewVerificationRecord(report, sig)
	b, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "draft:", err)
		return 1
	}
	fmt.Fprintln(stdout, string(b))
	if record.Verified {
		return 0
	}
	return 1
}

// readBodyOrFinal returns the article body. It prefers the body file the
// manifest was computed over; if the user pointed at a final document and the
// body file is gone, it strips the frontmatter from the final instead.
func readBodyOrFinal(bodyPath, givenPath string) (string, error) {
	if b, err := os.ReadFile(bodyPath); err == nil {
		return strings.TrimSpace(string(b)), nil
	}
	if strings.HasSuffix(givenPath, "-final.md") {
		b, err := os.ReadFile(givenPath)
		if err != nil {
			return "", err
		}
		_, body := frontmatter.Split(string(b))
		return strings.TrimSpace(body), nil
	}
	return "", fmt.Errorf("cannot read the article body at %s", bodyPath)
}

// resolveSourceDigest hashes a source file if it is still where the manifest
// recorded it, so --verify can confirm the inputs as well as the output.
func resolveSourceDigest(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", false
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

func printVerifyReport(w io.Writer, path, manifestPath string, r provenance.CheckReport) {
	head := func(s string) string { return brand.Title.Render(s) }
	dim := func(s string) string { return brand.Help.Render(s) }
	row := func(status, label, detail string) { fmt.Fprintf(w, "  %-3s %-20s %s\n", status, label, dim(detail)) }
	ok, bad, note := "ok", "!!", "--"

	fmt.Fprintf(w, "%s\n", head("PROVENANCE"))
	if len(r.Problems) > 0 {
		for _, p := range r.Problems {
			row(bad, "manifest", p)
		}
		fmt.Fprintln(w, "\n  "+brand.Accent.Render("Cannot verify.")+" The manifest could not be read.")
		return
	}
	row(ok, "made by", fmt.Sprintf("%s %s", firstNonEmpty(r.Generator.Name, "draft"), r.Generator.Version))
	if r.Grounding.Engine != "" {
		row(note, "written with", strings.TrimSpace(r.Grounding.Engine+" "+r.Grounding.Model))
	}
	if r.DigestMatches {
		row(ok, "article", "unchanged since it was written ("+shortHash(r.BodySHA256)+")")
	} else {
		row(bad, "article", "does NOT match the manifest — it was edited after the provenance was written")
	}
	if r.LedgerChecked {
		if r.LedgerMatches {
			row(ok, "claim ledger", "matches the verified claims")
		} else {
			row(bad, "claim ledger", "does not match the recorded digest")
		}
	}
	fmt.Fprintf(w, "\n%s\n", head("GROUNDING"))
	g := r.Grounding
	row(note, "claims", fmt.Sprintf("%d verified", len(g.Claims)))
	attributed := fmt.Sprintf("%d of %d sentences rest on a claim", g.Attributed, g.Sentences)
	if g.SentencesWithUngroundedNumbers > 0 {
		attributed += fmt.Sprintf("; %d carry a figure no claim contains", g.SentencesWithUngroundedNumbers)
	}
	row(note, "attribution", attributed)

	if len(r.Sources) > 0 {
		fmt.Fprintf(w, "\n%s\n", head("SOURCES"))
		for _, s := range r.Sources {
			switch {
			case !s.Found:
				row(note, filepath.Base(s.Path), "not on this machine (provenance still checks)")
			case s.Matches:
				row(ok, filepath.Base(s.Path), "unchanged since it was read")
			default:
				row(bad, filepath.Base(s.Path), "differs from the source that was read")
			}
		}
	}
}

// signatureFor validates a detached C2PA credential beside the set, if one is
// present, and returns its state. It returns nil when no credential accompanies
// the article. When c2patool is not installed the credential is reported as
// present-but-unchecked rather than failed: the digest checks still stand, and
// a verifier that can check the signature still can.
func signatureFor(bodyPath, manifestPath string) *provenance.SignatureState {
	sidecarPath := strings.TrimSuffix(manifestPath, ".json") + ".c2pa"
	sc, err := os.ReadFile(sidecarPath)
	if err != nil {
		return nil
	}
	if !c2paAvail() {
		return &provenance.SignatureState{Present: true, State: "unchecked: c2patool not installed"}
	}
	rep, err := c2paVerify(context.Background(), bodyPath, sc)
	if err != nil {
		return &provenance.SignatureState{Present: true, Checked: true, State: "error: " + err.Error()}
	}
	return &provenance.SignatureState{
		Present: true, Checked: true, Valid: rep.SignatureValid, Trusted: rep.Trusted, State: rep.State,
	}
}

// printSignatureReport prints a SIGNATURE section for a credential's state and
// returns whether it is acceptable for the overall verdict. A valid-but-
// untrusted credential (a development certificate) is acceptable and reported
// as such; a present-but-unchecked credential is noted but not a failure; only
// a checked-and-invalid signature fails.
func printSignatureReport(w io.Writer, sig *provenance.SignatureState) bool {
	dim := func(s string) string { return brand.Help.Render(s) }
	row := func(status, label, detail string) { fmt.Fprintf(w, "  %-3s %-20s %s\n", status, label, dim(detail)) }
	fmt.Fprintf(w, "\n%s\n", brand.Title.Render("SIGNATURE"))
	switch {
	case !sig.Checked:
		row("--", "credential", "signed credential present; install c2patool to verify its signature")
		return true
	case !sig.Valid:
		row("!!", "signature", "INVALID — the credential does not match this article")
		return false
	case sig.Trusted:
		row("ok", "signature", "valid and trusted")
	default:
		row("--", "signature", "valid; signing certificate not in a known trust list (e.g. a development certificate)")
	}
	return true
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
