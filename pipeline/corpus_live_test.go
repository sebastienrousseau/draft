// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/prompt"
)

// Live recall measurement against the grounding corpus.
//
// claims.TestGroundingCorpus pins the *gate* using recorded model output, so
// it catches a change to verification. It cannot catch the other half of the
// risk: a reworded extraction prompt that quietly makes the model return
// fewer usable claims. Nothing about that is visible in a diff, and the cost
// only shows up as thinner articles weeks later.
//
// This asks a real backend to extract from the same corpus sources and
// compares what survives against baseline-live.json — a recording of what a
// known-good prompt achieved on the same sources.
//
// It deliberately does NOT compare against the `verified` counts in
// corpus.json. Those describe crafted extractions (paraphrases, invented
// numbers, invalid schema) recorded to pin the gate; a real model given the
// same source produces good output and verifies more, so that comparison
// measures nothing. Getting this wrong once produced a cheerful "189% recall".
//
// It is a measurement, not a gate: model output is not deterministic and
// baselines drift as models change, so a tight threshold would fire on noise.
// It fails only on a collapse — see recallCollapseRatio — which is the shape a
// bad prompt edit actually has.
//
//	DRAFT_CORPUS_LIVE=1 make corpus-live
//
// Skipped by default: it costs one model call per case and needs a backend.
const (
	corpusDir = "../claims/testdata/corpus"
	// A prompt edit that halves recall is a regression whatever the noise.
	recallCollapseRatio = 0.5
)

func TestCorpusLiveRecall(t *testing.T) {
	if os.Getenv("DRAFT_CORPUS_LIVE") == "" {
		t.Skip("set DRAFT_CORPUS_LIVE=1 to measure live recall (costs one model call per case)")
	}

	cases := loadCorpusManifest(t)
	cfg := config.Load(config.Flags{})
	if err := engine.Validate(cfg); err != nil {
		t.Fatalf("engine configuration: %v", err)
	}
	chain := engine.ChainFor(cfg, engine.KindExtract)
	if len(chain) == 0 {
		t.Skip("no extraction backend available")
	}
	backend := chain[0]
	t.Logf("extracting with %s (%s)", backend.Name(), engine.ResolveModel(cfg, backend))

	baseline := loadLiveBaseline(t)
	if baseline == nil {
		t.Log("no baseline-live.json; this run will report counts without comparing")
	}

	var liveTotal, baselineTotal int
	for _, tc := range cases {
		source, err := os.ReadFile(filepath.Join(corpusDir, tc.Name+".source.md"))
		if err != nil {
			t.Fatalf("case %s: %v", tc.Name, err)
		}

		res, err := backend.Generate(context.Background(), engine.Request{
			Kind:        engine.KindExtract,
			Prompt:      prompt.Claim(string(source)),
			Temperature: extractTemperature,
		})
		if err != nil {
			t.Fatalf("case %s: extraction failed: %v", tc.Name, err)
		}

		records, dropped := claims.Parse(res.Text, string(source))
		liveTotal += len(records)

		want, known := baseline[tc.Name]
		baselineTotal += want
		status := "  "
		if known && len(records) < want {
			status = "! "
		}
		t.Logf("%s%-30s live %d verified / %d dropped   baseline %d",
			status, tc.Name, len(records), dropped, want)
	}

	if baselineTotal == 0 {
		t.Logf("live extraction produced %d verified claim(s); record them in "+
			"baseline-live.json to make future runs comparable", liveTotal)
		return
	}

	ratio := float64(liveTotal) / float64(baselineTotal)
	t.Logf("live recall: %d verified against a baseline of %d (%.0f%%)",
		liveTotal, baselineTotal, ratio*100)

	if ratio < recallCollapseRatio {
		t.Errorf("live recall collapsed to %.0f%% of the baseline. A prompt or "+
			"model change has cost most of the extractable claims; compare "+
			"prompt.Claim against the version that produced baseline-live.json "+
			"(engine %s, recorded then).", ratio*100, backend.Name())
	}
}

// loadLiveBaseline reads the recorded live counts, or nil when there are none.
func loadLiveBaseline(t *testing.T) map[string]int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(corpusDir, "baseline-live.json"))
	if err != nil {
		return nil
	}
	var b struct {
		Cases map[string]int `json:"cases"`
	}
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatalf("parsing baseline-live.json: %v", err)
	}
	return b.Cases
}

// corpusEntry mirrors the manifest claims/corpus_test.go reads. It is
// duplicated rather than exported: the manifest is test data, and giving it a
// public Go type would make a fixture format part of the library's API.
type corpusEntry struct {
	Name     string `json:"name"`
	Verified int    `json:"verified"`
}

func loadCorpusManifest(t *testing.T) []corpusEntry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(corpusDir, "corpus.json"))
	if err != nil {
		t.Fatalf("reading the corpus manifest: %v", err)
	}
	var m struct {
		Cases []corpusEntry `json:"cases"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing the corpus manifest: %v", err)
	}
	return m.Cases
}

// A corpus case referenced by the live harness but missing from disk would
// make the measurement quietly incomplete.
func TestCorpusManifestMatchesDisk(t *testing.T) {
	for _, tc := range loadCorpusManifest(t) {
		for _, suffix := range []string{".source.md", ".extraction.md"} {
			if _, err := os.Stat(filepath.Join(corpusDir, tc.Name+suffix)); err != nil {
				t.Errorf("manifest names %s but %s is missing", tc.Name, suffix)
			}
		}
	}
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Fatal(err)
	}
	var sources int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".source.md") {
			sources++
		}
	}
	if got := len(loadCorpusManifest(t)); sources != got {
		t.Errorf("%d source files on disk but %d cases in the manifest; "+
			"a case was added without being registered", sources, got)
	}
}
