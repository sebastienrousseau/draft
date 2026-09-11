// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package c2pa

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeTool installs a stub c2patool (a POSIX shell script) and points the
// package at it. The script's behaviour is driven by env vars the test sets, so
// the Go glue around c2patool — argument construction, reading the produced
// sidecar, the verify-after-sign check, and every error branch — is exercised
// without the real binary. The signature cryptography itself is covered by the
// live test against real c2patool.
func fakeTool(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake c2patool is a POSIX shell script")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-c2patool.sh")
	body := `#!/bin/sh
case "$*" in
  *--external-manifest*) printf '%s' "$FAKE_VERIFY_JSON"; exit 0 ;;
esac
# sign mode: locate the -o output and write the sidecar beside it.
out=""; prev=""
for a in "$@"; do [ "$prev" = "-o" ] && out="$a"; prev="$a"; done
[ -n "$FAKE_NO_SIDECAR" ] && exit 1
printf 'fake-credential' > "${out%.*}.c2pa"
exit 1
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := tool
	tool = script
	t.Cleanup(func() { tool = orig })
}

const validJSON = `{"validation_state":"Valid","validation_results":{"activeManifest":{"success":[{"code":"claimSignature.validated"}],"failure":[{"code":"signingCredential.untrusted"}]}}}`

func writeArticle(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.md")
	if err := os.WriteFile(p, []byte("# A\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSignHappyGlue(t *testing.T) {
	fakeTool(t)
	t.Setenv("FAKE_VERIFY_JSON", validJSON)
	cred, err := Sign(context.Background(), writeArticle(t), []byte(`{"assertions":[]}`), Signer{CertPath: "c.pem", KeyPath: "k.pem"})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if string(cred) != "fake-credential" {
		t.Errorf("credential = %q", cred)
	}
}

func TestSignNoCredentialProduced(t *testing.T) {
	fakeTool(t)
	t.Setenv("FAKE_NO_SIDECAR", "1")
	if _, err := Sign(context.Background(), writeArticle(t), []byte(`{}`), Signer{CertPath: "c", KeyPath: "k"}); err == nil {
		t.Error("expected an error when no sidecar is produced")
	}
}

func TestSignCredentialDoesNotVerify(t *testing.T) {
	fakeTool(t)
	t.Setenv("FAKE_VERIFY_JSON", `{"validation_state":"Invalid","validation_results":{"activeManifest":{"failure":[{"code":"assertion.dataHash.match"}]}}}`)
	if _, err := Sign(context.Background(), writeArticle(t), []byte(`{}`), Signer{CertPath: "c", KeyPath: "k"}); err == nil {
		t.Error("expected an error when the produced credential does not verify")
	}
}

func TestSignRejectsBadManifest(t *testing.T) {
	fakeTool(t)
	if _, err := Sign(context.Background(), writeArticle(t), []byte("not json"), Signer{CertPath: "c", KeyPath: "k"}); err == nil {
		t.Error("expected an error for a non-JSON manifest")
	}
}

func TestVerifyToolError(t *testing.T) {
	// A tool that is not installed makes Verify (and run) error cleanly.
	orig := tool
	tool = "definitely-no-such-binary-xyz"
	t.Cleanup(func() { tool = orig })
	if Available() {
		t.Skip("unexpected binary on PATH")
	}
	if _, err := Verify(context.Background(), writeArticle(t), []byte("x")); err == nil {
		t.Error("expected an error when c2patool is unavailable")
	}
}

func TestParseReportFlatStatus(t *testing.T) {
	// Older c2patool: a flat validation_status list plus validation_state.
	rep, err := parseReport(`{"validation_state":"Valid","validation_status":[{"code":"claimSignature.validated"},{"code":"signingCredential.untrusted"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.SignatureValid || rep.Trusted {
		t.Errorf("flat status: valid=%v trusted=%v, want true/false", rep.SignatureValid, rep.Trusted)
	}
}
