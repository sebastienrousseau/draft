// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package c2pa signs a draft article's provenance manifest into a detached
// C2PA credential and verifies one, by shelling out to c2patool.
//
// draft's manifest is a definition until someone signs it. This turns it into
// a signed sidecar (.c2pa) bound to the article's bytes, so a reader can
// confirm not just that the digests match (draft --verify already does that)
// but that the credential was issued by the holder of a specific certificate
// and has not been altered. Signing is opt-in: it runs only when a certificate
// and key are configured and c2patool is installed; the keyless, unsigned
// manifest remains the default.
//
// C2PA targets media assets, and c2patool reports a plain-text or Markdown
// article as an "unsupported file type" even as it writes a valid sidecar for
// it. So Sign does not trust c2patool's exit status: it confirms the credential
// by verifying it, and treats a valid signature over the article as the only
// success condition.
package c2pa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// tool is the c2patool binary name; a variable so a test can point at a stub.
var tool = "c2patool"

// Signer names the signing material: PEM files for the certificate chain
// (leaf first, up to a root) and its private key, and the signature algorithm
// c2patool should use (es256, es384, es512, ps256, ed25519, ...).
type Signer struct {
	CertPath string
	KeyPath  string
	Alg      string
}

// DefaultAlg is used when a Signer leaves Alg empty.
const DefaultAlg = "es256"

// Available reports whether c2patool is on PATH. Signing and signature
// verification are skipped, not failed, when it is not: provenance without a
// signature is still useful, and a missing optional tool must not fail a run.
func Available() bool {
	_, err := exec.LookPath(tool)
	return err == nil
}

// Report is the outcome of verifying a credential against an article.
type Report struct {
	// SignatureValid is true when c2patool validated the claim signature and
	// every hashed assertion and the data hash matched the article.
	SignatureValid bool
	// Trusted is true when the signing certificate chained to a trust anchor
	// c2patool recognises. A development certificate is valid but untrusted,
	// which is reported, not treated as a signature failure.
	Trusted bool
	// State is c2patool's overall validation_state ("Valid", "Invalid", ...).
	State string
	// Failures are the validation failure codes, minus the untrusted-credential
	// code that Trusted already conveys.
	Failures []string
}

// Sign turns a manifest definition into a signed sidecar credential for the
// article at articlePath and returns the credential bytes. It augments the
// definition with the signer's algorithm, key and certificate, asks c2patool
// to produce a sidecar, then verifies that sidecar against the article and
// returns an error unless the signature is valid — so a credential is only ever
// returned when it actually verifies.
func Sign(ctx context.Context, articlePath string, manifestDef []byte, s Signer) ([]byte, error) {
	cert, err := filepath.Abs(s.CertPath)
	if err != nil {
		return nil, fmt.Errorf("c2pa: resolving certificate path: %w", err)
	}
	key, err := filepath.Abs(s.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("c2pa: resolving key path: %w", err)
	}
	def, err := withSigner(manifestDef, cert, key, s.alg())
	if err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp("", "draft-c2pa-")
	if err != nil {
		return nil, fmt.Errorf("c2pa: temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, def, 0o600); err != nil {
		return nil, fmt.Errorf("c2pa: writing manifest: %w", err)
	}

	// c2patool requires the output extension to match the input, and writes the
	// sidecar as <stem>.c2pa beside it.
	out := filepath.Join(dir, filepath.Base(articlePath))
	// The exit status is deliberately ignored: c2patool errors on a text asset
	// as "unsupported file type" while still writing a valid sidecar. The
	// verify step below is what actually decides success.
	combined, _ := run(ctx, articlePath, "-m", manifestPath, "-s", "-o", out, "--no_signing_verify", "-f")

	sidecar := strings.TrimSuffix(out, filepath.Ext(out)) + ".c2pa"
	data, err := os.ReadFile(sidecar)
	if err != nil {
		return nil, fmt.Errorf("c2pa: c2patool produced no credential: %w; output: %s", err, strings.TrimSpace(combined))
	}
	rep, err := Verify(ctx, articlePath, data)
	if err != nil {
		return nil, err
	}
	if !rep.SignatureValid {
		return nil, fmt.Errorf("c2pa: the produced credential did not verify (state %q, failures: %s)", rep.State, strings.Join(rep.Failures, ", "))
	}
	return data, nil
}

// Verify checks a detached credential against the article at articlePath and
// reports whether its signature and hashes are valid and whether its
// certificate is trusted.
func Verify(ctx context.Context, articlePath string, sidecar []byte) (Report, error) {
	dir, err := os.MkdirTemp("", "draft-c2pa-verify-")
	if err != nil {
		return Report{}, fmt.Errorf("c2pa: temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	sidecarPath := filepath.Join(dir, "credential.c2pa")
	if err := os.WriteFile(sidecarPath, sidecar, 0o600); err != nil {
		return Report{}, fmt.Errorf("c2pa: writing credential: %w", err)
	}
	out, err := run(ctx, "--external-manifest", sidecarPath, articlePath)
	if err != nil {
		return Report{}, fmt.Errorf("c2pa: verify failed: %w; output: %s", err, strings.TrimSpace(out))
	}
	return parseReport(out)
}

// withSigner returns the manifest definition with the signer fields c2patool
// reads (alg, private_key, sign_cert) merged in, preserving everything else.
func withSigner(manifestDef []byte, cert, key, alg string) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(manifestDef, &m); err != nil {
		return nil, fmt.Errorf("c2pa: manifest is not a JSON object: %w", err)
	}
	m["alg"] = alg
	m["private_key"] = key
	m["sign_cert"] = cert
	return json.Marshal(m)
}

// parseReport reads c2patool's JSON report into a Report. The untrusted-
// credential code is not a signature failure: it means the certificate is not
// in a recognised trust list, which is exactly the state of a development
// certificate, so it sets Trusted=false rather than SignatureValid=false.
func parseReport(out string) (Report, error) {
	var doc struct {
		ValidationState   string `json:"validation_state"`
		ValidationResults struct {
			ActiveManifest struct {
				Success []statusEntry `json:"success"`
				Failure []statusEntry `json:"failure"`
			} `json:"activeManifest"`
		} `json:"validation_results"`
		// Older c2patool reports a flat validation_status list instead.
		ValidationStatus []statusEntry `json:"validation_status"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		return Report{}, fmt.Errorf("c2pa: could not read c2patool report: %w", err)
	}

	rep := Report{State: doc.ValidationState, Trusted: true}
	sawValidated := false
	note := func(code string) {
		if code == "claimSignature.validated" {
			sawValidated = true
		}
		if code == "signingCredential.untrusted" {
			rep.Trusted = false
		}
	}
	for _, s := range doc.ValidationResults.ActiveManifest.Success {
		note(s.Code)
	}
	for _, s := range doc.ValidationStatus {
		note(s.Code)
	}
	for _, f := range doc.ValidationResults.ActiveManifest.Failure {
		if f.Code == "signingCredential.untrusted" {
			rep.Trusted = false
			continue
		}
		if f.Code == "" || isInformational(f.Code) {
			continue
		}
		rep.Failures = append(rep.Failures, f.Code)
	}

	// A valid overall state is authoritative; absent that, a validated claim
	// signature with no recorded failure is the pass condition. An explicit
	// Invalid state always fails.
	rep.SignatureValid = rep.State != "Invalid" &&
		(rep.State == "Valid" || (sawValidated && len(rep.Failures) == 0))
	return rep, nil
}

type statusEntry struct {
	Code        string `json:"code"`
	Explanation string `json:"explanation"`
}

// isInformational reports whether a code is advisory rather than a failure of
// the signature or its bindings.
func isInformational(code string) bool {
	return strings.HasPrefix(code, "ingredient.")
}

func (s Signer) alg() string {
	if strings.TrimSpace(s.Alg) == "" {
		return DefaultAlg
	}
	return s.Alg
}

// run invokes c2patool with args and returns its combined output. A missing
// binary is reported as a clear error rather than a generic exec failure.
func run(ctx context.Context, args ...string) (string, error) {
	if !Available() {
		return "", errors.New("c2patool is not installed")
	}
	cmd := exec.CommandContext(ctx, tool, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
