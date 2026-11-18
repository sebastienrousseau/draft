// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/c2pa"
	"github.com/sebastienrousseau/draft/internal/c2pa/c2patest"
	"github.com/sebastienrousseau/draft/provenance"
)

// writeManifest writes a manifest as saveProvenance does and returns its
// bytes, so the signer is exercised on the real shape it must mark.
func writeManifest(t *testing.T, path string) []byte {
	t.Helper()
	b, _ := json.MarshalIndent(provenance.NewManifest(provenance.ManifestInput{Title: "x", Body: "b\n"}), "", "  ")
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return b
}

// signedAt reports whether the manifest at path records a signed credential.
func signedAt(t *testing.T, path string) bool {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := provenance.ParseManifest(b)
	if err != nil {
		t.Fatal(err)
	}
	return g.Signed
}

// With no signing certificate configured, signManifest writes nothing and the
// manifest stays an unsigned definition — the long-standing default.
func TestSignManifestNoopWithoutCert(t *testing.T) {
	dir := t.TempDir()
	r := NewRunner(config.Config{}, []engine.Engine{&countingEngine{name: "x"}}, nil)
	r.manifestPath = filepath.Join(dir, "m-c2pa.json")
	_ = os.WriteFile(r.manifestPath, []byte(`{"assertions":[]}`), 0o644)

	r.signManifest(context.Background(), dir, "m", filepath.Join(dir, "body.md"))
	if _, err := os.Stat(filepath.Join(dir, "provenance", "m-c2pa.c2pa")); !os.IsNotExist(err) {
		t.Error("a sidecar was written despite no signing certificate")
	}
}

// End-to-end: with a dev certificate configured, save produces a signed sidecar
// beside the set that verifies against the body file.
func TestSaveWritesVerifiableSignature(t *testing.T) {
	if !c2pa.Available() {
		t.Skip("c2patool not installed")
	}
	dir := t.TempDir()
	certPath, keyPath := c2patest.MustChain(dir)
	cfg := config.Config{
		HomeDir: dir, DraftsDir: dir,
		SignCert: certPath, SignKey: keyPath,
	}
	r := NewRunner(cfg, []engine.Engine{&countingEngine{name: "x"}}, nil)

	outputDir := filepath.Join(dir, "day")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	article := "# A Title\n\nOne grounded sentence about a result.\n"
	if _, _, err := r.save(context.Background(), outputDir, article, nil); err != nil {
		t.Fatal(err)
	}

	// Find the produced sidecar and body, and confirm the credential verifies.
	sidecars, _ := filepath.Glob(filepath.Join(outputDir, "provenance", "*-c2pa.c2pa"))
	if len(sidecars) != 1 {
		t.Fatalf("expected one signed sidecar, found %v", sidecars)
	}
	bodies, _ := filepath.Glob(filepath.Join(outputDir, "source", "*-body.md"))
	if len(bodies) != 1 {
		t.Fatalf("expected one body file, found %v", bodies)
	}
	sc, err := os.ReadFile(sidecars[0])
	if err != nil {
		t.Fatal(err)
	}
	rep, err := c2pa.Verify(context.Background(), bodies[0], sc)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.SignatureValid {
		t.Errorf("the sidecar save wrote does not verify: state %q failures %v", rep.State, rep.Failures)
	}
	manifests, _ := filepath.Glob(filepath.Join(outputDir, "provenance", "*-c2pa.json"))
	if len(manifests) != 1 {
		t.Fatalf("expected one manifest, found %v", manifests)
	}
	if !signedAt(t, manifests[0]) {
		t.Error("the manifest beside a signed credential does not record it")
	}
	m, _ := os.ReadFile(manifests[0])
	if why := provenance.Unbound(m, rep.Assertions); why != "" {
		t.Errorf("the credential does not vouch for the manifest beside it: %s", why)
	}
}

func TestSignManifestWritesSidecarWithFakeSigner(t *testing.T) {
	origA, origS := c2paAvailable, c2paSign
	defer func() { c2paAvailable, c2paSign = origA, origS }()
	c2paAvailable = func() bool { return true }
	c2paSign = func(context.Context, string, []byte, c2pa.Signer) ([]byte, error) { return []byte("cred"), nil }

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "provenance"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(config.Config{SignCert: "c", SignKey: "k"}, []engine.Engine{&countingEngine{name: "x"}}, nil)
	r.manifestPath = filepath.Join(dir, "m-c2pa.json")
	writeManifest(t, r.manifestPath)
	var signed []byte
	c2paSign = func(_ context.Context, _ string, m []byte, _ c2pa.Signer) ([]byte, error) {
		signed = m
		return []byte("cred"), nil
	}

	r.signManifest(context.Background(), dir, "m", filepath.Join(dir, "body.md"))
	b, err := os.ReadFile(filepath.Join(dir, "provenance", "m-c2pa.c2pa"))
	if err != nil || string(b) != "cred" {
		t.Errorf("sidecar = %q, err %v; want the signed credential", b, err)
	}
	// The credential signs a manifest that records it, and that manifest is
	// the one left beside the article.
	if !signedAt(t, r.manifestPath) {
		t.Error("the manifest does not record its signed credential")
	}
	if onDisk, _ := os.ReadFile(r.manifestPath); !bytes.Equal(onDisk, signed) {
		t.Error("the manifest on disk is not the one the credential signed")
	}
}

// A manifest the signer cannot mark is left as it was and nothing is signed:
// a credential over an unmarked manifest could be deleted unnoticed.
func TestSignManifestRefusesAnUnmarkableManifest(t *testing.T) {
	origA, origS := c2paAvailable, c2paSign
	defer func() { c2paAvailable, c2paSign = origA, origS }()
	c2paAvailable = func() bool { return true }
	called := false
	c2paSign = func(context.Context, string, []byte, c2pa.Signer) ([]byte, error) {
		called = true
		return []byte("cred"), nil
	}
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "provenance"), 0o755)
	r := NewRunner(config.Config{SignCert: "c", SignKey: "k"}, []engine.Engine{&countingEngine{name: "x"}}, nil)
	r.manifestPath = filepath.Join(dir, "m-c2pa.json")
	_ = os.WriteFile(r.manifestPath, []byte(`{"assertions":[]}`), 0o644)
	r.signManifest(context.Background(), dir, "m", filepath.Join(dir, "body.md"))
	if called {
		t.Error("a manifest with no grounding was signed")
	}
	if b, _ := os.ReadFile(r.manifestPath); string(b) != `{"assertions":[]}` {
		t.Errorf("the manifest was changed: %s", b)
	}
}

func TestSignManifestWarnsWhenToolMissing(t *testing.T) {
	origA := c2paAvailable
	defer func() { c2paAvailable = origA }()
	c2paAvailable = func() bool { return false }
	r := NewRunner(config.Config{SignCert: "c", SignKey: "k"}, []engine.Engine{&countingEngine{name: "x"}}, nil)
	r.manifestPath = "irrelevant"
	// Must not panic and must not write a sidecar.
	r.signManifest(context.Background(), t.TempDir(), "m", "body.md")
}

func TestSignManifestNoManifestPath(t *testing.T) {
	origA := c2paAvailable
	defer func() { c2paAvailable = origA }()
	c2paAvailable = func() bool { return true }
	r := NewRunner(config.Config{SignCert: "c", SignKey: "k"}, []engine.Engine{&countingEngine{name: "x"}}, nil)
	r.manifestPath = "" // nothing to sign
	r.signManifest(context.Background(), t.TempDir(), "m", "body.md")
}

func TestSignManifestReadError(t *testing.T) {
	origA := c2paAvailable
	defer func() { c2paAvailable = origA }()
	c2paAvailable = func() bool { return true }
	r := NewRunner(config.Config{SignCert: "c", SignKey: "k"}, []engine.Engine{&countingEngine{name: "x"}}, nil)
	r.manifestPath = filepath.Join(t.TempDir(), "does-not-exist.json")
	r.signManifest(context.Background(), t.TempDir(), "m", "body.md") // warns, no panic
}

func TestSignManifestSignError(t *testing.T) {
	origA, origS := c2paAvailable, c2paSign
	defer func() { c2paAvailable, c2paSign = origA, origS }()
	c2paAvailable = func() bool { return true }
	c2paSign = func(context.Context, string, []byte, c2pa.Signer) ([]byte, error) {
		return nil, errTest
	}
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "provenance"), 0o755)
	r := NewRunner(config.Config{SignCert: "c", SignKey: "k"}, []engine.Engine{&countingEngine{name: "x"}}, nil)
	r.manifestPath = filepath.Join(dir, "m-c2pa.json")
	writeManifest(t, r.manifestPath)
	r.signManifest(context.Background(), dir, "m", filepath.Join(dir, "body.md"))
	if _, err := os.Stat(filepath.Join(dir, "provenance", "m-c2pa.c2pa")); !os.IsNotExist(err) {
		t.Error("a sidecar was written despite a signing error")
	}
	if signedAt(t, r.manifestPath) {
		t.Error("the manifest records a credential that was never written")
	}
}

func TestSignManifestWriteSidecarError(t *testing.T) {
	origA, origS := c2paAvailable, c2paSign
	defer func() { c2paAvailable, c2paSign = origA, origS }()
	c2paAvailable = func() bool { return true }
	c2paSign = func(context.Context, string, []byte, c2pa.Signer) ([]byte, error) {
		return []byte("cred"), nil
	}
	dir := t.TempDir()
	provFile := filepath.Join(dir, "provenance")
	if err := os.WriteFile(provFile, []byte("blocker"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(config.Config{SignCert: "c", SignKey: "k"}, []engine.Engine{&countingEngine{name: "x"}}, nil)
	r.manifestPath = filepath.Join(dir, "m-c2pa.json")
	writeManifest(t, r.manifestPath)
	r.signManifest(context.Background(), dir, "m", filepath.Join(dir, "body.md"))
	// The credential and the marked manifest are written together or not at
	// all: a manifest that records a credential with none beside it fails
	// --verify.
	if signedAt(t, r.manifestPath) {
		t.Error("the manifest records a credential that could not be written")
	}
}
