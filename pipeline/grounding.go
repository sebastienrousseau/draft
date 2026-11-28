// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/extractcache"
	"github.com/sebastienrousseau/draft/internal/pdf"
)

// Prompt budgets for the compact writing ledger.
const (
	maxPromptClaims     = 45
	maxPromptClaimChars = 14000
	extractTemperature  = 0.15
	writeTemperature    = 0.6
	editTemperature     = 0.3
	// maxStemAttempts bounds the search for a free output filename.
	maxStemAttempts = 1000
)

// resumeOrExtract reuses a verified ledger from an earlier attempt when
// --resume is set and one is on disk, and otherwise mines the claims afresh.
//
// Extraction is 80-95% of a run's wall clock. When the write phase fails, that
// work was previously thrown away even though the ledger it produced was still
// sitting in the day folder — so a retry re-paid ten minutes for work that had
// already succeeded.
//
// The reused ledger is re-verified against the freshly sectioned sources rather
// than taken on trust, and to the same standard a fresh run applies: each
// record against one section, then the second gate when it is enabled.
// Resume therefore cannot weaken grounding: if a source changed underneath,
// the records it no longer supports are dropped here, and a wholly changed
// source falls back to a fresh extraction; a run that still verifies nothing
// stops with ErrNoVerifiedClaims before anything is written.
//
// xchain is the resolved extraction chain, passed in explicitly so the
// grounding phase never reaches into the Runner's per-kind chain map: the gate
// is a function of the sections, the chain it is handed, and the config.
func (r *Runner) resumeOrExtract(ctx context.Context, job Job, sections []pdf.Section, outputDir string, xchain *chainState) ([]claims.Record, int, error) {
	if !r.cfg.Resume {
		return r.extractClaims(ctx, job, sections, outputDir, xchain)
	}

	ledgerPath, found := resumableLedger(outputDir, job)
	var data []byte
	err := os.ErrNotExist
	if found {
		data, err = os.ReadFile(ledgerPath)
	}
	if err != nil {
		// No ledger to resume from is not an error: fall through to a normal
		// run rather than refusing to work.
		r.log("no ledger to resume from; extracting claims")
		return r.extractClaims(ctx, job, sections, outputDir, xchain)
	}

	bodies := make([]string, len(sections))
	for i, s := range sections {
		bodies[i] = s.Body
	}
	records, dropped := claims.ParseLedgerSections(string(data), bodies)
	if len(records) == 0 {
		r.warn("the saved ledger no longer verifies against these sources; extracting afresh")
		return r.extractClaims(ctx, job, sections, outputDir, xchain)
	}

	r.log(fmt.Sprintf("resumed %d claim(s) from %s", len(records), shortPath(r.cfg, ledgerPath)))
	if dropped > 0 {
		r.warn(fmt.Sprintf("dropped %d resumed claim(s) the sources no longer support", dropped))
	}
	// The run writes the ledger it uses under its own name rather than
	// rewriting the one it read, which another resume may be reading too.
	r.resumedFrom = ledgerPath
	return r.finishLedger(ctx, r.ownLedgerPath(outputDir, job), records, dropped)
}

// finishLedger is the end of the claims gate, shared by a fresh extraction and
// a resume so the two cannot drift: de-duplication, the opt-in second gate,
// then the ledger write. It cannot fail the run; a ledger that will not save
// is a warning.
func (r *Runner) finishLedger(ctx context.Context, path string, records []claims.Record, dropped int) ([]claims.Record, int, error) {
	if deduped := claims.Dedupe(records); len(deduped) != len(records) {
		r.log(fmt.Sprintf("removed %d duplicate claim(s)", len(records)-len(deduped)))
		records = deduped
	}
	// Opt-in semantic second pass: a model judges whether each verified quote
	// actually supports its claim. Off by default and fail-open, so it can only
	// tighten the ledger, never weaken it.
	records = r.secondGate(ctx, records)
	r.saveLedger(path, records, dropped)
	return records, dropped, nil
}

// sections reads and splits every source file. A source that cannot be read is
// skipped with a note rather than failing the run, but if nothing at all could
// be read the reason is returned — and when every failure was a missing text
// layer, the returned error preserves pdf.ErrNoTextLayer so the caller can
// surface its OCR advice instead of a generic "no text" message.
func (r *Runner) sections(ctx context.Context, sources []string) ([]pdf.Section, error) {
	var all []pdf.Section
	var firstErr error
	scanned := 0
	failed := 0

	for _, src := range sources {
		if sum, err := fileSHA256(src); err == nil {
			r.sourceDigests = append(r.sourceDigests, SourceDigest{Path: src, SHA256: sum})
		}
		text, err := pdf.ExtractWith(ctx, src, r.cfg.Reader)
		if err != nil {
			// Cancellation is not a skippable per-file problem.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			failed++
			if errors.Is(err, pdf.ErrNoTextLayer) {
				scanned++
			}
			if firstErr == nil {
				firstErr = err
			}
			r.warn(fmt.Sprintf("skipped %s: %v", filepath.Base(src), err))
			continue
		}
		all = append(all, pdf.SplitSections(filepath.Base(src), text)...)
	}

	if len(all) == 0 && failed > 0 {
		if scanned == failed {
			// Every source was a scan: %w keeps errors.Is working for callers
			// and keeps the remediation text in the message.
			return nil, fmt.Errorf("no readable text extracted from the source(s): %w", pdf.ErrNoTextLayer)
		}
		return nil, fmt.Errorf("no readable text extracted from the source(s): %w", firstErr)
	}
	return all, nil
}

// extractClaims mines quote-verified claims from every section. The first
// section runs through the engine chain to settle on a working backend; the
// remaining sections then run concurrently on that backend when it is a session
// provider (independent subprocess per call), or sequentially for Ollama (a
// single local model that should not be hit in parallel). Any section that
// fails a parallel call is retried through the chain, so a mid-run provider drop
// still degrades to Ollama.
func (r *Runner) extractClaims(ctx context.Context, job Job, sections []pdf.Section, outputDir string, xchain *chainState) ([]claims.Record, int, error) {
	raw := make([]string, len(sections))

	r.cacheHits.Store(0)
	extract := func(body string) (string, error) { return r.extractOne(ctx, body, nil, xchain) }

	// A declined section is a section with no claims. The model has said it
	// will not restate this text; the rest of the paper is still there, and
	// the ledger stays honest because nothing is invented to fill the gap.
	refused := func(i int, err error) bool {
		if !errors.Is(err, engine.ErrRefused) {
			return false
		}
		r.warn(fmt.Sprintf("claim section %d/%d: %v; recorded as having no claims", i+1, len(sections), err))
		return true
	}

	// Section 0 settles the engine via the chain.
	r.log(fmt.Sprintf("claim section 1/%d", len(sections)))
	text0, err := extract(sections[0].Body)
	if err != nil && !refused(0, err) {
		return nil, 0, fmt.Errorf("claim extraction failed: %w", err)
	}
	raw[0] = text0

	// Pin the backend section 0 settled on. Parallel workers call it directly
	// rather than going through the chain, so they cannot race on the cursor.
	conc := r.extractConcurrency(xchain)
	pinned := xchain.active()
	if pinned == nil {
		return nil, 0, errors.New("no extraction engine available")
	}
	if conc > 1 && len(sections) > 1 {
		r.log(fmt.Sprintf("extracting %d section(s) with %d workers via %s", len(sections)-1, conc, pinned.Name()))
	}

	var failed []int
	if conc > 1 {
		failed = r.extractParallel(ctx, sections, raw, pinned, conc, xchain, refused)
	} else if err := r.extractSequential(sections, raw, extract, refused); err != nil {
		return nil, 0, err
	}

	// Retry any parallel failures through the chain (handles a provider drop).
	sort.Ints(failed)
	for _, i := range failed {
		r.log(fmt.Sprintf("retrying claim section %d/%d", i+1, len(sections)))
		text, err := extract(sections[i].Body)
		if err != nil && !refused(i, err) {
			return nil, 0, fmt.Errorf("claim extraction failed: %w", err)
		}
		raw[i] = text
	}

	if hits := r.cacheHits.Load(); hits > 0 {
		r.log(fmt.Sprintf("reused %d cached extraction(s); use --no-cache to force a fresh run", hits))
	}

	records, dropped := r.assembleRecords(sections, raw)
	return r.finishLedger(ctx, r.ownLedgerPath(outputDir, job), records, dropped)
}

// extractConcurrency is the number of parallel extraction workers for the settled
// engine: the configured value for a session provider (independent subprocesses),
// and no more than engine.OllamaParallelism for Ollama, which is how many
// requests one local server is known to run at once.
func (r *Runner) extractConcurrency(xchain *chainState) int {
	n := r.cfg.ExtractConcurrency
	if n < 1 {
		n = 1
	}
	// Read the extraction chain rather than the last-used engine name: under
	// per-kind routing the writer may be a session provider while extraction
	// runs locally, and it is the local one that must not be over-driven.
	extractor := ""
	if e := xchain.active(); e != nil {
		extractor = e.Name()
	}
	if extractor == "ollama" {
		if limit := engine.OllamaParallelism(); n > limit {
			return limit
		}
	}
	return n
}

// extractOne returns a section's raw extraction, reusing a cached one when the
// same section, prompt, engine and model produced it before.
//
// direct names the backend to call straight through, which the parallel path
// uses so its workers cannot race on the chain cursor; nil routes through the
// chain, so a failure can still fall back.
//
// The cached text is never trusted on its own account: the caller still runs
// claims.Parse over it against the freshly read section, so a stale entry can
// only ever yield fewer verified claims, never an ungrounded one.
func (r *Runner) extractOne(ctx context.Context, body string, direct engine.Engine, xchain *chainState) (string, error) {
	serving := direct
	if serving == nil {
		serving = xchain.active()
	}
	req := extractRequest(body, serving, r.cfg.StructuredOutput)
	if key := r.extractKey(body, serving, req); key != "" {
		if text, ok := r.cache.Get(key); ok {
			r.cacheHits.Add(1)
			return text, nil
		}
	}

	var text string
	var err error
	if direct != nil {
		var res engine.Result
		res, err = direct.Generate(ctx, req)
		text = res.Text
		if err == nil {
			r.addUsage(res.Usage)
		}
	} else {
		text, err = r.generateTextOn(ctx, req, xchain)
	}
	if err != nil {
		return "", err
	}
	// A parallel worker calls its pinned engine directly, bypassing generate()
	// and tryAlternates where content refusals are otherwise caught. So a
	// worker detects the pinned engine's content refusal here and returns
	// ErrRefused; the caller then routes it to the alternates like any other.
	// The chain path (direct == nil) is handled inside generateOn(xchain).
	if direct != nil && contentRefusal(req, engine.Result{Text: text}) {
		return "", fmt.Errorf("%s: %w", serving.Name(), engine.ErrRefused)
	}

	// Address the entry to the backend that actually served it: the chain may
	// have failed over between the lookup above and this point, and storing
	// it under the wrong name would serve one model's output for another.
	actual := direct
	if actual == nil {
		actual = xchain.active()
	}
	if key := r.extractKey(body, actual, req); key != "" {
		if putErr := r.cache.Put(key, text); putErr != nil {
			r.warn("could not cache the extraction: " + putErr.Error())
		}
	}
	return text, nil
}

// contentRefusal reports whether a successful engine result is really the
// model declining an extraction request in prose. It is scoped to extraction,
// where the CLAIM/NONE schema makes a decline unambiguous; a writing response
// is long free prose and is never judged this way.
func contentRefusal(req engine.Request, res engine.Result) bool {
	return req.Kind == engine.KindExtract && looksLikeExtractionRefusal(res.Text)
}

// looksLikeExtractionRefusal reports whether an extraction response is the
// model declining the task rather than answering it. The extraction prompt
// asks for CLAIM: records or exactly NONE, so a response with neither, short
// enough to be a refusal and carrying refusal language, is treated as one.
//
// It is deliberately conservative. An empty response is a different failure,
// not a refusal. A response containing a CLAIM: line produced records. NONE is
// the legitimate "no claims here". A long blob without CLAIM: is a malformed
// extraction the ledger step already drops to nothing, not a refusal. Only a
// short, schema-free response with an explicit refusal phrase qualifies, so a
// genuine extraction is never mistaken for a decline.
func looksLikeExtractionRefusal(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" || len(t) > 400 {
		return false
	}
	upper := strings.ToUpper(t)
	if strings.Contains(upper, "CLAIM:") || upper == "NONE" || strings.HasPrefix(t, "[") {
		return false
	}
	low := strings.ToLower(t)
	for _, marker := range refusalMarkers {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

// refusalMarkers are phrases a model uses when it declines. They are matched
// as substrings of a lower-cased response, so contractions and both persons
// ("I can't", "I am unable") are covered.
var refusalMarkers = []string{
	"i can't", "i cannot", "i can not", "i won't", "i will not",
	"i'm unable", "i am unable", "i'm not able", "i am not able",
	"i'm sorry", "i am sorry", "i apologize", "i apologise",
	"can't help", "cannot help", "can't assist", "cannot assist",
	"unable to help", "unable to assist", "unable to provide", "unable to complete",
	"i must decline", "i'd rather not", "i would rather not",
	"i don't feel comfortable", "i do not feel comfortable",
	"cannot comply", "can't comply", "will not comply", "against my guidelines",
}

// extractAlternates asks each engine behind the extraction chain's active one
// for a section the active engine declined. It returns the first answer, or
// the original refusal when every alternate declines or fails.
func (r *Runner) extractAlternates(ctx context.Context, body string, refusal error, xchain *chainState) (string, error) {
	cs := xchain
	if cs == nil || cs.cur >= len(cs.engines) {
		return "", refusal
	}
	req := extractRequest(body, cs.active(), r.cfg.StructuredOutput)
	res, alt, ok := r.tryAlternates(ctx, cs, cs.cur, req)
	if !ok {
		return "", refusal
	}
	if key := r.extractKey(body, alt, req); key != "" {
		if putErr := r.cache.Put(key, res.Text); putErr != nil {
			r.warn("could not cache the extraction: " + putErr.Error())
		}
	}
	return res.Text, nil
}

// extractKey addresses one section's extraction against a specific backend
// and the prompt variant req carried, since the text and JSON prompts
// produce different output for the same section.
func (r *Runner) extractKey(body string, e engine.Engine, req engine.Request) string {
	if e == nil || r.cache == nil {
		return ""
	}
	return extractcache.Key(body, r.cfg.Reader, extractVersion(req), e.Name(), engine.ResolveModel(r.cfg, e))
}
