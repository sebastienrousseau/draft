// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/pdf"
)

// countingEngine records how many generation calls it served.
type countingEngine struct {
	name  string
	calls atomic.Int64
	out   string
}

func (c *countingEngine) Name() string { return c.name }
func (c *countingEngine) Generate(context.Context, engine.Request) (engine.Result, error) {
	c.calls.Add(1)
	return engine.Result{Text: c.out}, nil
}

func cacheTestSections() []pdf.Section {
	return []pdf.Section{
		{Label: "a", Body: "The system reached 12 pages per second in testing."},
		{Label: "b", Body: "A second section with entirely different wording here."},
	}
}

// Extraction is 80-95% of a run's wall clock, and the only reuse before this
// was a ledger named for today's date and the source's filename.
func TestExtractionIsReusedAcrossRuns(t *testing.T) {
	cacheDir := t.TempDir()
	sections := cacheTestSections()
	eng := &countingEngine{name: "fake", out: "NONE"}

	run := func(outDir string) *Runner {
		cfg := config.Config{HomeDir: outDir, DraftsDir: outDir, CacheDir: cacheDir}
		r := NewRunner(cfg, []engine.Engine{eng}, nil)
		if _, _, err := r.extractClaims(context.Background(), Job{Sources: []string{"p.pdf"}}, sections, outDir); err != nil {
			t.Fatal(err)
		}
		return r
	}

	first := run(t.TempDir())
	if got := eng.calls.Load(); got != int64(len(sections)) {
		t.Fatalf("first run made %d calls, want %d", got, len(sections))
	}
	if got := first.cacheHits.Load(); got != 0 {
		t.Errorf("first run reported %d cache hits, want 0", got)
	}

	// A different output directory stands in for a different day: the old
	// date-keyed ledger could not have been reused here.
	second := run(t.TempDir())
	if got := eng.calls.Load(); got != int64(len(sections)) {
		t.Errorf("second run made %d further call(s); the cache was not used", got-int64(len(sections)))
	}
	if got := second.cacheHits.Load(); got != int64(len(sections)) {
		t.Errorf("second run reported %d cache hits, want %d", got, len(sections))
	}
}

// Every input that can change the output must miss.
func TestCacheMissesWhenTheBackendChanges(t *testing.T) {
	cacheDir := t.TempDir()
	sections := cacheTestSections()

	warm := &countingEngine{name: "first", out: "NONE"}
	cfg := config.Config{HomeDir: t.TempDir(), CacheDir: cacheDir}
	dir := t.TempDir()
	r := NewRunner(cfg, []engine.Engine{warm}, nil)
	if _, _, err := r.extractClaims(context.Background(), Job{}, sections, dir); err != nil {
		t.Fatal(err)
	}

	other := &countingEngine{name: "second", out: "NONE"}
	r2 := NewRunner(cfg, []engine.Engine{other}, nil)
	if _, _, err := r2.extractClaims(context.Background(), Job{}, sections, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := other.calls.Load(); got != int64(len(sections)) {
		t.Errorf("a different engine served %d calls, want %d — the key ignores the backend", got, len(sections))
	}
	if got := r2.cacheHits.Load(); got != 0 {
		t.Errorf("a different engine reported %d cache hits, want 0", got)
	}
}

func TestCacheDisabledWhenNoCacheDirIsSet(t *testing.T) {
	sections := cacheTestSections()
	eng := &countingEngine{name: "fake", out: "NONE"}
	cfg := config.Config{HomeDir: t.TempDir()} // CacheDir empty

	for i := 0; i < 2; i++ {
		r := NewRunner(cfg, []engine.Engine{eng}, nil)
		if _, _, err := r.extractClaims(context.Background(), Job{}, sections, t.TempDir()); err != nil {
			t.Fatal(err)
		}
		if got := r.cacheHits.Load(); got != 0 {
			t.Errorf("run %d reported %d hits with caching disabled", i, got)
		}
	}
	if got := eng.calls.Load(); got != int64(2*len(sections)) {
		t.Errorf("engine served %d calls, want %d", got, 2*len(sections))
	}
}

// A cached entry is not trusted on its own account: the caller still verifies
// it against the freshly read section, so a stale entry can only ever yield
// fewer claims, never an ungrounded one.
func TestCachedExtractionIsStillVerifiedAgainstTheSource(t *testing.T) {
	cacheDir := t.TempDir()
	// An extraction whose quote does not occur in the section it is replayed
	// against — exactly what a source edited between runs would produce.
	forged := "CLAIM: the system hit 99 pages per second\n" +
		"SOURCE_QUOTE: \"the system hit 99 pages per second\"\nTYPE: metric\nSTRENGTH: demonstrated\n---\n"
	eng := &countingEngine{name: "fake", out: forged}
	cfg := config.Config{HomeDir: t.TempDir(), CacheDir: cacheDir}

	sections := []pdf.Section{{Label: "a", Body: "the system hit 99 pages per second in testing"}}
	r := NewRunner(cfg, []engine.Engine{eng}, nil)
	records, _, err := r.extractClaims(context.Background(), Job{}, sections, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected the claim to verify against its own section, got %d", len(records))
	}

	// Replay the same cached text against a section that no longer supports it.
	changed := []pdf.Section{{Label: "a", Body: "the system hit 99 pages per second in testing"}}
	changed[0].Body = "an entirely different sentence with no such measurement"
	r2 := NewRunner(cfg, []engine.Engine{eng}, nil)
	records2, dropped, err := r2.extractClaims(context.Background(), Job{}, changed, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(records2) != 0 {
		t.Errorf("a claim survived against a section that does not contain its quote: %+v", records2)
	}
	if dropped == 0 {
		t.Error("the unsupported claim should have been counted as dropped")
	}
}

func TestCacheWriteFailureWarnsButDoesNotFailTheRun(t *testing.T) {
	// A cache directory that is really a file: Open succeeds only if MkdirAll
	// does, so use a path whose shard cannot be created instead.
	dir := t.TempDir()
	cfg := config.Config{HomeDir: t.TempDir(), CacheDir: dir}
	sections := cacheTestSections()
	eng := &countingEngine{name: "fake", out: "NONE"}

	events := make(chan Event, 64)
	r := NewRunner(cfg, []engine.Engine{eng}, events)
	// Occupy every shard path the run will need with a plain file.
	for _, sec := range sections {
		key := r.extractKey(sec.Body, eng)
		if err := writeFileAt(dir, key[:2]); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := r.extractClaims(context.Background(), Job{}, sections, t.TempDir()); err != nil {
		t.Fatalf("a cache write failure must not fail the run: %v", err)
	}
	close(events)
	var warned bool
	for e := range events {
		if w, ok := e.(WarnEvent); ok && strings.Contains(string(w), "could not cache") {
			warned = true
		}
	}
	if !warned {
		t.Error("expected a warning about the failed cache write")
	}
}

func writeFileAt(dir, name string) error {
	return os.WriteFile(filepath.Join(dir, name), nil, 0o600)
}
