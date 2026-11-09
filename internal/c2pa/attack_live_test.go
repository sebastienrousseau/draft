// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package c2pa

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/internal/c2pa/c2patest"
)

// attackManifest is a manifest definition carrying its own data hash: one
// exclusion over the whole article, so the hash is of nothing. c2patool keeps
// a hard binding it is handed rather than computing one, which is all the
// exclusion-range attack needs. The hash is given as a byte list because
// c2patool decodes the assertion as CBOR bytes.
func attackManifest(t *testing.T, articleLen int) []byte {
	t.Helper()
	empty := sha256.Sum256(nil)
	hash := make([]int, len(empty))
	for i, b := range empty {
		hash[i] = int(b)
	}
	def, err := json.Marshal(map[string]any{
		"claim_generator_info": []map[string]string{{"name": "draft", "version": "0.0.0"}},
		"title":                "article.md",
		"format":               "text/markdown",
		"assertions": []map[string]any{{
			"label": "c2pa.hash.data",
			"data": map[string]any{
				"name":       "jumbf manifest",
				"alg":        "sha256",
				"hash":       hash,
				"pad":        []int{},
				"exclusions": []map[string]int{{"start": 0, "length": articleLen}},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return def
}

// Buchanan's exclusion-range attack (October 2026), built with real
// c2patool rather than by editing a report: the credential is signed over
// one article and then presented with a forgery of the same length, which
// c2patool accepts as Valid. draft must not, and must not sign it either.
func TestExclusionAttackLive(t *testing.T) {
	if !Available() {
		t.Skip("c2patool not installed")
	}
	dir := t.TempDir()
	certPath, keyPath := c2patest.MustChain(dir)
	article := filepath.Join(dir, "article.md")
	signed := []byte("# Title\n\nWhat the signer wrote.\n")
	if err := os.WriteFile(article, signed, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	signer := Signer{CertPath: certPath, KeyPath: keyPath}

	if _, err := Sign(ctx, article, attackManifest(t, len(signed)), signer); err == nil || !strings.Contains(err.Error(), "excludes 1 byte range") {
		t.Fatalf("Sign handed back an exclusion credential: %v", err)
	}

	sidecar := forge(t, ctx, article, attackManifest(t, len(signed)), signer)
	forged := filepath.Join(dir, "forged.md")
	if err := os.WriteFile(forged, []byte("# Title\n\nWhat an attacker wrote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Verify(ctx, forged, sidecar)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if rep.State != "Valid" {
		// The check below still has to hold; this only says the forgery no
		// longer shows what it was built to show.
		t.Logf("c2patool itself now rejects the forgery (state %q)", rep.State)
	}
	if rep.SignatureValid || !strings.Contains(rep.Uncovered, "excludes 1 byte range") {
		t.Errorf("a forged article verified under an exclusion credential: %+v", rep)
	}
}

// forge signs the attack credential the way Sign does, minus Sign's own
// verification, which refuses it.
func forge(t *testing.T, ctx context.Context, article string, manifest []byte, s Signer) []byte {
	t.Helper()
	def, err := withSigner(manifest, s.CertPath, s.KeyPath, s.alg())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, def, 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "article.md")
	combined, _ := run(ctx, article, "-m", manifestPath, "-s", "-o", out, "--no_signing_verify", "-f")
	sidecar, err := os.ReadFile(filepath.Join(dir, "article.c2pa"))
	if err != nil {
		t.Fatalf("c2patool produced no credential: %v; output: %s", err, combined)
	}
	return sidecar
}
