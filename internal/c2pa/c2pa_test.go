// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package c2pa

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sebastienrousseau/draft/internal/c2pa/c2patest"
)

// --- parseReport: deterministic, no c2patool needed (runs everywhere) ---

func TestParseReportValidUntrusted(t *testing.T) {
	// The real shape c2patool emits for a dev-cert-signed asset: valid
	// signature and hashes, credential untrusted.
	out := `{
      "validation_state": "Valid",
      "validation_results": {
        "activeManifest": {
          "success": [
            {"code": "claimSignature.validated"},
            {"code": "assertion.dataHash.match"}
          ],
          "failure": [
            {"code": "signingCredential.untrusted", "explanation": "signing certificate untrusted"}
          ]
        }
      }
    }`
	rep, err := parseReport(out)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.SignatureValid {
		t.Error("valid signature should be reported valid")
	}
	if rep.Trusted {
		t.Error("a dev credential must be reported untrusted")
	}
	if rep.State != "Valid" {
		t.Errorf("state = %q, want Valid", rep.State)
	}
	if len(rep.Failures) != 0 {
		t.Errorf("untrusted is not a signature failure, got %v", rep.Failures)
	}
}

func TestParseReportInvalidSignature(t *testing.T) {
	out := `{
      "validation_state": "Invalid",
      "validation_results": {
        "activeManifest": {
          "success": [],
          "failure": [
            {"code": "assertion.dataHash.match", "explanation": "hash mismatch"}
          ]
        }
      }
    }`
	rep, err := parseReport(out)
	if err != nil {
		t.Fatal(err)
	}
	if rep.SignatureValid {
		t.Error("a hash mismatch must not report a valid signature")
	}
	found := false
	for _, f := range rep.Failures {
		if f == "assertion.dataHash.match" {
			found = true
		}
	}
	if !found {
		t.Errorf("the hash failure should be recorded, got %v", rep.Failures)
	}
}

func TestParseReportGarbage(t *testing.T) {
	if _, err := parseReport("not json"); err == nil {
		t.Error("garbage output should error")
	}
}

func TestWithSignerMergesFields(t *testing.T) {
	got, err := withSigner([]byte(`{"title":"t","assertions":[]}`), "/c.pem", "/k.pem", "es256")
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, want := range []string{`"sign_cert":"/c.pem"`, `"private_key":"/k.pem"`, `"alg":"es256"`, `"title":"t"`} {
		if !contains(s, want) {
			t.Errorf("merged manifest missing %s: %s", want, s)
		}
	}
	if _, err := withSigner([]byte("not json"), "c", "k", "es256"); err == nil {
		t.Error("non-JSON manifest should error")
	}
}

func TestSignerAlgDefault(t *testing.T) {
	if (Signer{}).alg() != DefaultAlg {
		t.Errorf("empty alg = %q, want %q", (Signer{}).alg(), DefaultAlg)
	}
	if (Signer{Alg: "es384"}).alg() != "es384" {
		t.Error("explicit alg not honoured")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- live sign + verify against real c2patool (skips when absent) ---

func TestSignAndVerifyLive(t *testing.T) {
	if !Available() {
		t.Skip("c2patool not installed")
	}
	dir := t.TempDir()
	certPath, keyPath := c2patest.MustChain(dir)

	article := filepath.Join(dir, "article.md")
	if err := os.WriteFile(article, []byte("# Title\n\nA grounded sentence.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"claim_generator_info":[{"name":"draft","version":"0.0.0"}],` +
		`"title":"article.md","format":"text/markdown",` +
		`"assertions":[{"label":"com.draftlib.grounding","data":{"grounded":true}}]}`)

	ctx := context.Background()
	sidecar, err := Sign(ctx, article, manifest, Signer{CertPath: certPath, KeyPath: keyPath})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if len(sidecar) == 0 {
		t.Fatal("empty credential")
	}

	rep, err := Verify(ctx, article, sidecar)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !rep.SignatureValid {
		t.Errorf("signature should be valid, got state %q failures %v", rep.State, rep.Failures)
	}
	if rep.Trusted {
		t.Error("a dev-cert credential should be reported untrusted, not trusted")
	}

	// Tamper: the same credential must not verify against a different article.
	other := filepath.Join(dir, "other.md")
	if err := os.WriteFile(other, []byte("# Different\n\nTampered.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep2, err := Verify(ctx, other, sidecar)
	if err != nil {
		t.Fatalf("Verify(tampered): %v", err)
	}
	if rep2.SignatureValid {
		t.Error("a credential must not verify against a different article")
	}
}
