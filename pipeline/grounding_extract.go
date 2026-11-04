// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/internal/pdf"
)

// extractParallel runs sections 1..n on the pinned backend with conc workers,
// filling raw and returning the indices that failed for a retry through the
// chain.
func (r *Runner) extractParallel(ctx context.Context, sections []pdf.Section, raw []string, pinned engine.Engine, conc int, xchain *chainState, refused func(int, error) bool) []int {
	var mu sync.Mutex
	var failed []int
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for i := 1; i < len(sections); i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			text, err := r.extractOne(ctx, sections[i].Body, pinned, xchain)
			if errors.Is(err, engine.ErrRefused) {
				// Workers bypass the chain, so the chain's own routing
				// for a refusal never runs here. Offer the section to
				// the engines behind the pinned one ourselves; the
				// cursor stays put either way.
				text, err = r.extractAlternates(ctx, sections[i].Body, err, xchain)
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if !refused(i, err) {
					failed = append(failed, i)
				}
				return
			}
			raw[i] = text
		}(i)
	}
	wg.Wait()
	return failed
}

// extractSequential runs sections 1..n one at a time, filling raw.
func (r *Runner) extractSequential(sections []pdf.Section, raw []string, extract func(string) (string, error), refused func(int, error) bool) error {
	// Sequential extraction is where a run actually spends its time, so it
	// is where an estimate is worth having: without one there is no way to
	// tell a two-minute run from a twenty-minute one until it ends.
	eta := newETA(len(sections), time.Since(r.started))
	for i := 1; i < len(sections); i++ {
		r.log(fmt.Sprintf("claim section %d/%d%s", i+1, len(sections), eta.remaining(i)))
		started := time.Now()
		text, err := extract(sections[i].Body)
		if err != nil && !refused(i, err) {
			return fmt.Errorf("claim extraction failed: %w", err)
		}
		eta.observe(time.Since(started))
		raw[i] = text
	}
	return nil
}

// assembleRecords parses each section's raw extraction into verified records
// and adds the claims mined from its tables, returning the records and the
// number of claims dropped for failing quote verification.
func (r *Runner) assembleRecords(sections []pdf.Section, raw []string) ([]claims.Record, int) {
	var records []claims.Record
	dropped := 0
	tableClaims := 0
	for i, sec := range sections {
		secRecords, secDropped := claims.Parse(raw[i], sec.Body)
		records = append(records, secRecords...)
		dropped += secDropped
		// A layout reader (Docling) preserves tables as Markdown, where the
		// prose extraction cannot quote a cell. Mine those cells directly: the
		// value is read from the parsed cell and re-verified against the source
		// like any other claim, so this only ever adds grounded facts. Plain
		// text sources carry no Markdown tables, so this is a no-op for them.
		cells := claims.TableClaims(sec.Body)
		records = append(records, cells...)
		tableClaims += len(cells)
	}
	if tableClaims > 0 {
		r.log(fmt.Sprintf("mined %d claim(s) from tables", tableClaims))
	}
	return records, dropped
}
