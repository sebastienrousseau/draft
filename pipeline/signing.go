// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"os"
	"path/filepath"

	"github.com/sebastienrousseau/draft/internal/c2pa"
)

// signManifest turns the just-written manifest definition into a signed,
// detached C2PA credential beside the set, when a signing certificate is
// configured and c2patool is installed. It binds the credential to the body
// file — the same bytes draft --verify checks — so a verifier can confirm both
// the digests and the signature.
//
// Signing is opt-in and never fails a run: the article is already saved and
// grounded, and an unsigned manifest is the long-standing default. Every
// problem is a warning the reader sees, not a lost draft.
func (r *Runner) signManifest(ctx context.Context, outputDir, stem, bodyPath string) {
	if r.cfg.SignCert == "" || r.cfg.SignKey == "" {
		return // signing not configured: keyless, unsigned manifest (the default)
	}
	if !c2pa.Available() {
		r.warn("C2PA signing is configured but c2patool is not installed; wrote an unsigned manifest")
		return
	}
	if r.manifestPath == "" {
		return // no manifest definition was written to sign
	}
	manifest, err := os.ReadFile(r.manifestPath)
	if err != nil {
		r.warn("could not read the manifest to sign: " + err.Error())
		return
	}
	cred, err := c2pa.Sign(ctx, bodyPath, manifest, c2pa.Signer{
		CertPath: r.cfg.SignCert, KeyPath: r.cfg.SignKey, Alg: r.cfg.SignAlg,
	})
	if err != nil {
		r.warn("C2PA signing failed; wrote an unsigned manifest: " + err.Error())
		return
	}
	sidecar := filepath.Join(outputDir, "provenance", stem+"-c2pa.c2pa")
	if err := os.WriteFile(sidecar, cred, 0o644); err != nil {
		r.warn("could not write the signed credential: " + err.Error())
		return
	}
	r.log("signed manifest: " + shortPath(r.cfg, sidecar))
}
