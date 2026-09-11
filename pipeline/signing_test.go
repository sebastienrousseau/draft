// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/c2pa"
	"github.com/sebastienrousseau/draft/internal/c2pa/c2patest"
)

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
}
