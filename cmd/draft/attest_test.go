// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sebastienrousseau/draft/internal/c2pa"
	"github.com/sebastienrousseau/draft/provenance"
)

// hermeticAttest isolates a test from the user's config, signing variables
// and the clock.
func hermeticAttest(t *testing.T) {
	t.Helper()
	t.Setenv("DRAFT_NO_CONFIG", "1")
	for _, k := range []string{"DRAFT_C2PA_CERT", "DRAFT_C2PA_KEY", "DRAFT_C2PA_ALG"} {
		t.Setenv(k, "")
	}
	orig := attestNow
	t.Cleanup(func() { attestNow = orig })
	attestNow = func() time.Time { return time.Date(2026, 11, 8, 9, 30, 0, 0, time.UTC) }
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb strings.Builder
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func verifyJSON(t *testing.T, path string) provenance.VerificationRecord {
	t.Helper()
	var out, errb strings.Builder
	runVerifyJSON(path, &out, &errb)
	var rec provenance.VerificationRecord
	if err := json.Unmarshal([]byte(out.String()), &rec); err != nil {
		t.Fatalf("verify --json: %v\n%s%s", err, out.String(), errb.String())
	}
	return rec
}

// The Art. 50(4) case: a person edits draft's article and takes editorial
// responsibility for it. Before the review, --verify rightly calls the edit
// unverified; after it, the edit is accounted for, and the report still says
// the text is AI-generated and that the grounding describes draft's version.
func TestAttestAccountsForAReviewedEdit(t *testing.T) {
	hermeticAttest(t)
	day, stem := writeSet(t, "# Title\n\nThe original sentence.", false)
	body := filepath.Join(day, "source", stem+"-body.md")
	if err := os.WriteFile(body, []byte("# Title\n\nThe sentence, as the editor left it.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := runCLI(t, "--verify", body); code != 1 || !strings.Contains(out, "prompt_guided") {
		t.Fatalf("an unreviewed edit verified, or its oversight is missing: exit %d\n%s", code, out)
	}

	code, out, errs := runCLI(t, "--attest", body, "--reviewed-by", "Ada Example")
	if code != 0 || !strings.Contains(out, "review by Ada Example at 2026-11-08T09:30:00Z of an edited text") || !strings.Contains(out, "unsigned") {
		t.Fatalf("attest: exit %d\n%s%s", code, out, errs)
	}

	code, out, _ = runCLI(t, "--verify", body)
	for _, want := range []string{"AI-generated", "REVIEW", "Ada Example", "before the reviewer's edits", "Verified as reviewed. Ada Example", "human_validated"} {
		if code != 0 || !strings.Contains(out, want) {
			t.Errorf("verify after review: exit %d, missing %q\n%s", code, want, out)
		}
	}
	rec := verifyJSON(t, body)
	if !rec.Verified || !rec.Disclosure.AIGenerated || rec.Disclosure.HumanOversightLevel != provenance.OversightHumanValidated || rec.Review == nil || !rec.Review.Matches || !rec.Review.Edited || rec.Article.Matches {
		t.Errorf("record = %+v, review %+v", rec, rec.Review)
	}

	// An edit after the review is not covered by it.
	if err := os.WriteFile(body, []byte("# Title\n\nChanged again, unreviewed.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ = runCLI(t, "--verify", body)
	if code != 1 || !strings.Contains(out, "has changed since") {
		t.Errorf("an edit after the review verified: exit %d\n%s", code, out)
	}
}

func TestAttestOfTheTextDraftWrote(t *testing.T) {
	hermeticAttest(t)
	day, stem := writeSet(t, "# Title\n\nA grounded sentence.", false)
	final := filepath.Join(day, "final", stem+"-final.md")
	code, out, errs := runCLI(t, "--attest", final, "--reviewed-by", "ed-42")
	if code != 0 || !strings.Contains(out, "of the text draft wrote") {
		t.Fatalf("attest: exit %d\n%s%s", code, out, errs)
	}
	code, out, _ = runCLI(t, "--verify", final)
	if code != 0 || !strings.Contains(out, "Verified. The article matches") || strings.Contains(out, "before the reviewer's edits") {
		t.Errorf("verify: exit %d\n%s", code, out)
	}
	// A second review replaces the first.
	runCLI(t, "--attest", final, "--reviewed-by", "ed-43")
	if rec := verifyJSON(t, final); rec.Review == nil || rec.Review.Reviewer != "ed-43" {
		t.Errorf("review after re-attesting = %+v", rec.Review)
	}
}

// A review of other text, while the article is still draft's own, is
// reported but is not a failure: nothing about the article is unaccounted for.
func TestVerifyReportsAReviewOfOtherText(t *testing.T) {
	hermeticAttest(t)
	day, stem := writeSet(t, "# Title\n\nThe original sentence.", false)
	body := filepath.Join(day, "source", stem+"-body.md")
	original, _ := os.ReadFile(body)
	if err := os.WriteFile(body, []byte("# Title\n\nAn edit.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "--attest", body, "--reviewed-by", "Ada")
	if err := os.WriteFile(body, original, 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runCLI(t, "--verify", body)
	if code != 0 || !strings.Contains(out, "of a different text from this one") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

func TestAttestRefusals(t *testing.T) {
	hermeticAttest(t)
	day, stem := writeSet(t, "# Title\n\nA sentence.", false)
	body := filepath.Join(day, "source", stem+"-body.md")
	manifest := filepath.Join(day, "provenance", stem+"-c2pa.json")
	before, _ := os.ReadFile(manifest)

	if code, _, errs := runCLI(t, "--attest", body); code != 2 || !strings.Contains(errs, "--reviewed-by") {
		t.Errorf("no reviewer: exit %d, %q", code, errs)
	}
	if code, _, errs := runCLI(t, "--attest", filepath.Join(day, "notes.txt"), "--reviewed-by", "Ada"); code != 1 || !strings.Contains(errs, "not a draft set file") {
		t.Errorf("not a set: exit %d, %q", code, errs)
	}

	// A signed article without a signer: the credential would stop matching.
	if err := os.WriteFile(sidecarPath(manifest), []byte("cred"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := runCLI(t, "--attest", body, "--reviewed-by", "Ada"); code != 1 || !strings.Contains(errs, "signed credential") {
		t.Errorf("signed, no signer: exit %d, %q", code, errs)
	}
	if after, _ := os.ReadFile(manifest); !bytes.Equal(before, after) {
		t.Error("a refused attest changed the manifest")
	}

	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := runCLI(t, "--attest", body, "--reviewed-by", "Ada"); code != 1 || !strings.Contains(errs, "no manifest") {
		t.Errorf("no manifest: exit %d, %q", code, errs)
	}
	if err := os.Remove(body); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := runCLI(t, "--attest", body, "--reviewed-by", "Ada"); code != 1 || !strings.Contains(errs, "cannot read") {
		t.Errorf("no body: exit %d, %q", code, errs)
	}
	writeFile(t, manifest, "{}")
	writeFile(t, body, "# T\n")
	if code, _, errs := runCLI(t, "--attest", body, "--reviewed-by", "Ada"); code != 1 || !strings.Contains(errs, "assertion") {
		t.Errorf("foreign manifest: exit %d, %q", code, errs)
	}
}

func writeFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// With a signer configured the review is signed together with the manifest,
// and the credential is written only when signing succeeds.
func TestAttestSignsTheReview(t *testing.T) {
	hermeticAttest(t)
	t.Setenv("DRAFT_C2PA_CERT", "/certs/cert.pem")
	t.Setenv("DRAFT_C2PA_KEY", "/certs/key.pem")
	origA, origS := c2paAvail, attestSign
	t.Cleanup(func() { c2paAvail, attestSign = origA, origS })

	day, stem := writeSet(t, "# Title\n\nA sentence.", false)
	body := filepath.Join(day, "source", stem+"-body.md")
	manifest := filepath.Join(day, "provenance", stem+"-c2pa.json")

	c2paAvail = func() bool { return false }
	if code, _, errs := runCLI(t, "--attest", body, "--reviewed-by", "Ada"); code != 1 || !strings.Contains(errs, "c2patool is not installed") {
		t.Errorf("no c2patool: exit %d, %q", code, errs)
	}

	c2paAvail = func() bool { return true }
	attestSign = func(context.Context, string, []byte, c2pa.Signer) ([]byte, error) { return nil, errors.New("bad key") }
	if code, _, errs := runCLI(t, "--attest", body, "--reviewed-by", "Ada"); code != 1 || !strings.Contains(errs, "bad key") {
		t.Errorf("sign failure: exit %d, %q", code, errs)
	}
	if _, err := os.Stat(sidecarPath(manifest)); err == nil {
		t.Error("a failed signing wrote a credential")
	}

	var signed []byte
	var signer c2pa.Signer
	attestSign = func(_ context.Context, bodyPath string, m []byte, s c2pa.Signer) ([]byte, error) {
		if bodyPath != body {
			t.Errorf("signed over %s, want the body file", bodyPath)
		}
		signed, signer = m, s
		return []byte("credential"), nil
	}
	code, out, errs := runCLI(t, "--attest", body, "--reviewed-by", "Ada")
	if code != 0 || strings.Contains(out, "unsigned") {
		t.Fatalf("signed attest: exit %d\n%s%s", code, out, errs)
	}
	if !bytes.Contains(signed, []byte(provenance.ReviewLabel)) || signer.CertPath != "/certs/cert.pem" {
		t.Errorf("signer saw manifest without the review, or the wrong signer %+v", signer)
	}
	if got, _ := os.ReadFile(sidecarPath(manifest)); string(got) != "credential" {
		t.Errorf("credential = %q", got)
	}
	if got, _ := os.ReadFile(manifest); !bytes.Equal(got, signed) {
		t.Error("the manifest written is not the one signed")
	}
}
