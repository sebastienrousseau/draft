// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/provenance"
)

// The manifest is signed and meant to travel with the article. An absolute
// source path put the user's home directory, and with it usually their
// account name, into it.
func TestManifestRecordsSourcesRelativeToItself(t *testing.T) {
	cfg := testConfig(t)
	events := make(chan Event, 4096)
	runner := NewRunner(cfg, []engine.Engine{okEngine("claude")}, events)
	src := writeSource(t)
	dones, errs, _ := collect(t, runner, events, Job{Sources: []string{src}})
	if len(errs) != 0 || len(dones) != 1 {
		t.Fatalf("errs=%v dones=%d", errs, len(dones))
	}
	b, err := os.ReadFile(dones[0].ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := provenance.ParseManifest(b)
	if err != nil || len(g.Sources) != 1 {
		t.Fatalf("sources=%v err=%v", g.Sources, err)
	}
	recorded := g.Sources[0].Path
	if filepath.IsAbs(recorded) || path.IsAbs(recorded) || strings.Contains(recorded, `\`) {
		t.Fatalf("the manifest records %q; want a slash-separated relative path", recorded)
	}
	resolved := filepath.Join(filepath.Dir(dones[0].ManifestPath), filepath.FromSlash(recorded))
	if same, _ := sameFile(resolved, src); !same {
		t.Errorf("%q does not lead from the manifest back to the source %s", recorded, src)
	}
}

// Where no relative path exists, as between Windows volumes, the base name
// is recorded: it identifies the source without saying where it lived.
func TestManifestSourcePathFallsBackToTheBaseName(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "papers", "paper.pdf")
	if got := manifestSourcePath("relative/dir", abs); got != "paper.pdf" {
		t.Errorf("manifestSourcePath = %q, want the base name", got)
	}
}

func sameFile(a, b string) (bool, error) {
	ia, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	ib, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(ia, ib), nil
}
