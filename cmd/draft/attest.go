// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/internal/atomicfile"
	"github.com/sebastienrousseau/draft/internal/c2pa"
	"github.com/sebastienrousseau/draft/provenance"
)

// Seams so the attest path is testable without c2patool or a fixed clock.
var (
	attestSign = c2pa.Sign
	attestNow  = time.Now
)

// runAttest records in an article's manifest that a named person reviewed it
// as it now stands and takes editorial responsibility for it. With a signing
// certificate configured, the manifest is signed again so the review is
// covered by the credential; without one it is recorded unsigned, and said so.
func runAttest(o options, stdout, stderr io.Writer) int {
	if strings.TrimSpace(o.reviewedBy) == "" {
		fmt.Fprintln(stderr, "draft: --attest needs --reviewed-by <name or id>: the person taking editorial responsibility")
		return 2
	}
	cfg := config.Load(o.flags)
	for _, w := range cfg.Warnings {
		fmt.Fprintln(stderr, "draft: warning:", w)
	}
	rv, signed, manifestPath, err := attest(cfg, o.attestPath, o.reviewedBy)
	if err != nil {
		fmt.Fprintln(stderr, "draft: attest:", err)
		return 1
	}
	what := "the text draft wrote"
	if rv.Edited {
		what = "an edited text (the grounding figures describe draft's version)"
	}
	fmt.Fprintf(stdout, "draft: recorded a review by %s at %s of %s in %s\n", rv.Reviewer, rv.When, what, filepath.Base(manifestPath))
	if !signed {
		fmt.Fprintln(stdout, "draft: the review is unsigned, so it is a statement, not proof; set DRAFT_C2PA_CERT and DRAFT_C2PA_KEY to sign it")
	}
	return 0
}

// attest writes the review into the set's manifest, and a fresh credential
// beside it when it can sign. Both are written together or not at all, so a
// manifest never carries a review its credential does not cover.
func attest(cfg config.Config, path, reviewer string) (rv provenance.Review, signed bool, manifestPath string, err error) {
	bodyPath, manifestPath, err := locateSet(path)
	if err != nil {
		return rv, false, "", err
	}
	body, err := readBodyOrFinal(bodyPath, path)
	if err != nil {
		return rv, false, "", err
	}
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return rv, false, "", fmt.Errorf("no manifest for this article: %w", err)
	}
	// The same bytes --verify hashes, so the review is checked against what
	// it approved.
	updated, rv, err := provenance.Attest(manifest, body+"\n", reviewer, attestNow())
	if err != nil {
		return rv, false, "", err
	}
	files := map[string][]byte{manifestPath: updated}
	cred, err := signReview(cfg, bodyPath, manifestPath, updated)
	if err != nil {
		return rv, false, "", err
	}
	if cred != nil {
		files[sidecarPath(manifestPath)] = cred
	}
	return rv, cred != nil, manifestPath, atomicfile.WriteSet(files, 0o644)
}

// signReview signs the updated manifest when a signer is configured. Without
// one it refuses to touch a signed article: its credential would no longer
// match the manifest, and --verify would rightly call it invalid.
func signReview(cfg config.Config, bodyPath, manifestPath string, manifest []byte) ([]byte, error) {
	if cfg.SignCert == "" || cfg.SignKey == "" {
		if _, err := os.Stat(sidecarPath(manifestPath)); err == nil {
			return nil, errors.New("this article has a signed credential; set DRAFT_C2PA_CERT and DRAFT_C2PA_KEY so the review is signed as well")
		}
		return nil, nil
	}
	if !c2paAvail() {
		return nil, errors.New("signing is configured but c2patool is not installed")
	}
	return attestSign(context.Background(), bodyPath, manifest, c2pa.Signer{
		CertPath: cfg.SignCert, KeyPath: cfg.SignKey, Alg: cfg.SignAlg,
	})
}

// sidecarPath is the signed credential beside a manifest definition.
func sidecarPath(manifestPath string) string {
	return strings.TrimSuffix(manifestPath, ".json") + ".c2pa"
}
