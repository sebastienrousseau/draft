// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package c2pa

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sebastienrousseau/draft/internal/c2pa/c2patest"
	"github.com/sebastienrousseau/draft/provenance"
)

// provenance.Unbound compares draft's assertions with the copy a credential
// signed, which assumes c2patool stores them verbatim. This signs a real
// manifest, with a review and an AI disclosure, and holds the assumption to
// the installed c2patool: the credential vouches for the manifest it signed
// and not for one edited afterwards.
func TestSignedAssertionsBindTheManifestLive(t *testing.T) {
	if !Available() {
		t.Skip("c2patool not installed")
	}
	dir := t.TempDir()
	cert, key := c2patest.MustChain(dir)
	body := "# T\n\nA sentence about a result.\n"
	article := filepath.Join(dir, "a.md")
	if err := os.WriteFile(article, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.MarshalIndent(provenance.NewManifest(provenance.ManifestInput{
		Title: "x", Body: body, Engine: "claude",
		Sources: []provenance.Source{{Path: "p.pdf", SHA256: "ab"}},
	}), "", "  ")
	manifest, _, err := provenance.Attest(manifest, body, "Ada", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	cred, err := Sign(ctx, article, manifest, Signer{CertPath: cert, KeyPath: key})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Verify(ctx, article, cred)
	if err != nil {
		t.Fatal(err)
	}
	if why := provenance.Unbound(manifest, rep.Assertions); why != "" {
		t.Fatalf("a credential must vouch for the manifest it signed: %s", why)
	}
	forged := []byte(strings.Replace(string(manifest), `"sha256": "ab"`, `"sha256": "cd"`, 1))
	if string(forged) == string(manifest) {
		t.Fatal("fixture is wrong: no source digest to forge")
	}
	if why := provenance.Unbound(forged, rep.Assertions); !strings.Contains(why, provenance.GroundingLabel) {
		t.Errorf("a source digest edited after signing was not caught: %q", why)
	}
}
