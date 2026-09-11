// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/internal/c2pa"
	"github.com/sebastienrousseau/draft/internal/c2pa/c2patest"
	"github.com/sebastienrousseau/draft/provenance"
)

// writeSet lays out a minimal day-folder set with a body, a final document and
// a matching manifest, and returns the day root.
func writeSet(t *testing.T, body string, tamperManifest bool) (day, stem string) {
	t.Helper()
	day = t.TempDir()
	stem = "2026-09-06-a-grounded-article"
	for _, d := range []string{"source", "final", "provenance", "yaml"} {
		if err := os.MkdirAll(filepath.Join(day, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	saved := strings.TrimSpace(body) + "\n"
	must := func(p, content string) {
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(day, "source", stem+"-body.md"), saved)
	must(filepath.Join(day, "final", stem+"-final.md"), "---\ntitle: x\n---\n\n"+saved)

	att := provenance.Attribute(saved, nil)
	man := provenance.NewManifest(provenance.ManifestInput{Title: "x", Body: saved, Version: "0.0.34", Engine: "claude", Attribution: &att})
	mb, _ := json.MarshalIndent(man, "", "  ")
	if tamperManifest {
		mb = []byte(`{"assertions":[]}`)
	}
	must(filepath.Join(day, "provenance", stem+"-c2pa.json"), string(mb))
	return day, stem
}

func TestRunVerifyHappyPath(t *testing.T) {
	day, stem := writeSet(t, "# Title\n\nA grounded sentence about a result.", false)
	var out, errb strings.Builder
	for _, entry := range []string{
		filepath.Join(day, "final", stem+"-final.md"),
		filepath.Join(day, "source", stem+"-body.md"),
		filepath.Join(day, "provenance", stem+"-c2pa.json"),
	} {
		out.Reset()
		errb.Reset()
		if code := runVerify(entry, &out, &errb); code != 0 {
			t.Errorf("verify %s: exit %d, stderr %s", filepath.Base(entry), code, errb.String())
		}
		if !strings.Contains(out.String(), "Verified.") {
			t.Errorf("verify %s: output missing verdict:\n%s", filepath.Base(entry), out.String())
		}
	}
}

func TestRunVerifyDetectsAnEditedArticle(t *testing.T) {
	day, stem := writeSet(t, "# Title\n\nThe original sentence.", false)
	bodyPath := filepath.Join(day, "source", stem+"-body.md")
	if err := os.WriteFile(bodyPath, []byte("# Title\n\nThe original sentence, now edited.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb strings.Builder
	if code := runVerify(bodyPath, &out, &errb); code != 1 {
		t.Errorf("an edited body should fail with exit 1, got %d", code)
	}
	if !strings.Contains(out.String(), "does NOT match") || !strings.Contains(out.String(), "Not verified.") {
		t.Errorf("output should explain the mismatch:\n%s", out.String())
	}
}

// The final document can be verified even when the body file is gone: the
// frontmatter is stripped and the remaining body is what the digest covers.
func TestRunVerifyFallsBackToFinalWhenBodyMissing(t *testing.T) {
	day, stem := writeSet(t, "# Title\n\nA grounded sentence.", false)
	if err := os.Remove(filepath.Join(day, "source", stem+"-body.md")); err != nil {
		t.Fatal(err)
	}
	var out, errb strings.Builder
	if code := runVerify(filepath.Join(day, "final", stem+"-final.md"), &out, &errb); code != 0 {
		t.Errorf("final-only verify should pass, got %d: %s", code, errb.String())
	}
}

func TestRunVerifyErrors(t *testing.T) {
	day, stem := writeSet(t, "# Title\n\nBody.", false)
	cases := map[string]struct {
		path       string
		wantStderr string
	}{
		"not a set file":   {filepath.Join(day, "random.txt"), "not a draft set file"},
		"missing manifest": {filepath.Join(t.TempDir(), "source", "2026-09-06-x-body.md"), ""},
	}
	// Remove the manifest for the missing-manifest case's sibling lookup.
	if err := os.Remove(filepath.Join(day, "provenance", stem+"-c2pa.json")); err != nil {
		t.Fatal(err)
	}
	cases["manifest gone"] = struct {
		path       string
		wantStderr string
	}{filepath.Join(day, "final", stem+"-final.md"), "no manifest"}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var out, errb strings.Builder
			if code := runVerify(tc.path, &out, &errb); code != 1 {
				t.Errorf("expected exit 1, got %d", code)
			}
			if tc.wantStderr != "" && !strings.Contains(errb.String(), tc.wantStderr) {
				t.Errorf("stderr = %q, want %q", errb.String(), tc.wantStderr)
			}
		})
	}
}

// A manifest that cannot be parsed is reported, not verified.
func TestRunVerifyReportsAForeignManifest(t *testing.T) {
	day, stem := writeSet(t, "# Title\n\nBody.", true)
	var out, errb strings.Builder
	if code := runVerify(filepath.Join(day, "final", stem+"-final.md"), &out, &errb); code != 1 {
		t.Errorf("a foreign manifest should fail, got %d", code)
	}
	if !strings.Contains(out.String(), "Cannot verify.") {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestLocateSet(t *testing.T) {
	body, manifest, err := locateSet("/drafts/2026-09-06/final/2026-09-06-x-final.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(body, filepath.Join("2026-09-06", "source", "2026-09-06-x-body.md")) {
		t.Errorf("body = %s", body)
	}
	if !strings.HasSuffix(manifest, filepath.Join("provenance", "2026-09-06-x-c2pa.json")) {
		t.Errorf("manifest = %s", manifest)
	}
	if _, _, err := locateSet("/x/notaset.md"); err == nil {
		t.Error("a non-set filename must error")
	}
}

// writeRichSet builds a set whose manifest names a real source file and,
// optionally, leaves a kept ledger beside it, so --verify exercises the
// source-hashing and ledger-checking branches and the full report.
func writeRichSet(t *testing.T, sourceContent string, keepLedger bool, ungroundedNumber bool) (day, stem, sourcePath string) {
	t.Helper()
	day = t.TempDir()
	stem = "2026-09-06-rich"
	for _, d := range []string{"source", "final", "provenance"} {
		if err := os.MkdirAll(filepath.Join(day, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sourcePath = filepath.Join(t.TempDir(), "paper.pdf")
	if err := os.WriteFile(sourcePath, []byte(sourceContent), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "# Title\n\nThe system reached 0.82 on the test set."
	if ungroundedNumber {
		body += " Also 99 percent elsewhere."
	}
	saved := body + "\n"
	if err := os.WriteFile(filepath.Join(day, "source", stem+"-body.md"), []byte(saved), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(day, "final", stem+"-final.md"), []byte("---\nx: y\n---\n\n"+saved), 0o644); err != nil {
		t.Fatal(err)
	}
	ledgerBytes := []byte("# Verified Claim Ledger\n\nCLAIM: x\n")
	sum := sha256File(t, sourcePath)
	in := provenance.ManifestInput{
		Title: "Title", Body: saved, Version: "0.0.34", Engine: "claude", Model: "sonnet",
		Sources: []provenance.Source{{Path: sourcePath, SHA256: sum}},
	}
	if keepLedger {
		lsum := sha256Bytes(ledgerBytes)
		in.LedgerSHA256 = lsum
		if err := os.WriteFile(filepath.Join(day, stem+"-verified-claim-ledger.md"), ledgerBytes, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	att := provenance.Attribute(saved, nil)
	in.Attribution = &att
	man := provenance.NewManifest(in)
	mb, _ := json.MarshalIndent(man, "", "  ")
	if err := os.WriteFile(filepath.Join(day, "provenance", stem+"-c2pa.json"), mb, 0o644); err != nil {
		t.Fatal(err)
	}
	return day, stem, sourcePath
}

func TestRunVerifyChecksSourcesAndLedger(t *testing.T) {
	day, stem, sourcePath := writeRichSet(t, "the source text", true, true)
	final := filepath.Join(day, "final", stem+"-final.md")

	// Everything present and unchanged.
	var out, errb strings.Builder
	if code := runVerify(final, &out, &errb); code != 0 {
		t.Fatalf("should verify, exit %d: %s", code, errb.String())
	}
	for _, want := range []string{"unchanged since it was read", "matches the verified claims", "written with", "carry a figure no claim contains"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report missing %q:\n%s", want, out.String())
		}
	}

	// A changed source fails.
	if err := os.WriteFile(sourcePath, []byte("the source text, altered"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := runVerify(final, &out, &errb); code != 1 || !strings.Contains(out.String(), "differs from the source") {
		t.Errorf("a changed source should fail, exit %d:\n%s", code, out.String())
	}

	// An absent source is a note, not a failure.
	if err := os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := runVerify(final, &out, &errb); code != 0 || !strings.Contains(out.String(), "not on this machine") {
		t.Errorf("an absent source must not fail, exit %d:\n%s", code, out.String())
	}
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	got, ok := resolveSourceDigest(path)
	if !ok {
		t.Fatalf("hashing %s failed", path)
	}
	return got
}

func sha256Bytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestVerifyHelpers(t *testing.T) {
	if shortHash("abcdef0123456789") != "abcdef012345" {
		t.Errorf("shortHash long = %q", shortHash("abcdef0123456789"))
	}
	if shortHash("short") != "short" {
		t.Errorf("shortHash short = %q", shortHash("short"))
	}
	if firstNonEmpty("", "fallback") != "fallback" || firstNonEmpty("real", "x") != "real" || firstNonEmpty("  ", "b") != "b" {
		t.Error("firstNonEmpty")
	}
}

// A signed set verifies its signature: --verify reports the SIGNATURE section,
// accepts a valid dev-cert credential (untrusted but valid), and fails when the
// body is altered after signing.
func TestRunVerifySignature(t *testing.T) {
	if !c2paAvailable() {
		t.Skip("c2patool not installed")
	}
	day, stem := writeSet(t, "# Title\n\nA grounded sentence about a result.", false)
	certPath, keyPath := c2patest.MustChain(t.TempDir())
	bodyPath := filepath.Join(day, "source", stem+"-body.md")
	manifestPath := filepath.Join(day, "provenance", stem+"-c2pa.json")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	cred, err := c2paSign(bodyPath, manifest, certPath, keyPath)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sidecarPath := strings.TrimSuffix(manifestPath, ".json") + ".c2pa"
	if err := os.WriteFile(sidecarPath, cred, 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errb strings.Builder
	if code := runVerify(filepath.Join(day, "final", stem+"-final.md"), &out, &errb); code != 0 {
		t.Errorf("signed verify: exit %d, out %s", code, out.String())
	}
	if !strings.Contains(out.String(), "SIGNATURE") || !strings.Contains(out.String(), "valid") {
		t.Errorf("verify output missing signature section:\n%s", out.String())
	}

	// Alter the body after signing: the signature must now fail.
	if err := os.WriteFile(bodyPath, []byte("# Title\n\nAn altered sentence.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errb.Reset()
	if code := runVerify(bodyPath, &out, &errb); code != 1 {
		t.Errorf("tampered signed verify: exit %d, want 1\n%s", code, out.String())
	}
}

func c2paAvailable() bool { return c2pa.Available() }

func c2paSign(bodyPath string, manifest []byte, cert, key string) ([]byte, error) {
	return c2pa.Sign(context.Background(), bodyPath, manifest, c2pa.Signer{CertPath: cert, KeyPath: key})
}
