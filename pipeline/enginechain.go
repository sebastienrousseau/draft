// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"errors"
	"fmt"

	"github.com/sebastienrousseau/draft/engine"
)

// currentUsage returns a copy of the job's accumulated usage.
func (r *Runner) currentUsage() engine.Usage {
	r.usageMu.Lock()
	defer r.usageMu.Unlock()
	return r.usage
}

// generateText runs a request through the engine chain and returns its text.
func (r *Runner) generateText(ctx context.Context, req engine.Request) (string, error) {
	res, err := r.generate(ctx, req)
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

// addUsage accumulates one call's usage into the current job's total.
func (r *Runner) addUsage(u engine.Usage) {
	r.usageMu.Lock()
	r.usage.Add(u)
	r.usageMu.Unlock()
}

// generate runs a request against the active engine, advancing along the chain
// on error (a provider that is offline, not logged in, or failing) until one
// succeeds or the chain is exhausted. The advance is sticky: once an engine
// fails the run does not return to it, so a queue of sections is not re-attempted
// against a dead provider.
func (r *Runner) generate(ctx context.Context, req engine.Request) (engine.Result, error) {
	cs := r.chainFor(req.Kind)
	if cs.rehabilitate(timeNow()) {
		r.log("retrying " + cs.engines[0].Name() + " after its earlier failure")
		r.engineName = cs.engines[0].Name()
		r.emit(EngineEvent(r.engineName))
	}
	var lastErr error
	for cs != nil && cs.cur < len(cs.engines) {
		// A cancelled run must not keep walking the chain. Without this check
		// Ctrl+C makes every remaining engine fail in turn, each logging a
		// misleading "falling back to ..." on the way out.
		if err := ctx.Err(); err != nil {
			return engine.Result{}, err
		}
		e := cs.engines[cs.cur]
		res, err := e.Generate(ctx, req)
		if err == nil && contentRefusal(req, res) {
			// A provider with no typed stop reason declined in prose. Treat it
			// exactly as a typed refusal so the routing below offers the prompt
			// to the engines behind this one without demoting the chain.
			err = fmt.Errorf("%s: %w", e.Name(), engine.ErrRefused)
		}
		if err == nil {
			if r.engineName != e.Name() {
				r.engineName = e.Name()
				r.emit(EngineEvent(e.Name()))
			}
			r.addUsage(res.Usage)
			return res, nil
		}
		// Cancellation and timeout are the caller's decision, not a sick
		// backend. Failing over would retry work the user just abandoned.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return engine.Result{}, err
		}
		// A refusal is the model's verdict on this prompt, not on the backend.
		// Demoting the chain here downgrades every remaining job in the queue
		// because one section of one paper described something the model
		// would not touch, and with a single pinned engine it strands the
		// queue with nothing to fall back to. Keep the cursor where it is and
		// offer this one prompt to the engines behind it: another model may
		// well answer, and the next prompt goes back to the preferred one.
		if errors.Is(err, engine.ErrRefused) {
			if res, alt, ok := r.tryAlternates(ctx, cs, cs.cur, req); ok {
				if req.Kind == engine.KindWrite {
					r.wroteWith = alt
					r.emit(EngineEvent(alt.Name()))
				}
				return res, nil
			}
			return engine.Result{}, err
		}
		lastErr = err
		r.warn(fmt.Sprintf("%s failed (%v)", e.Name(), err))
		cs.cur++
		cs.demotedAt = timeNow()
		if cs.cur < len(cs.engines) {
			r.engineName = cs.engines[cs.cur].Name()
			r.emit(EngineEvent(r.engineName))
			r.warn("falling back to " + r.engineName)
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no engine available")
	}
	return engine.Result{}, lastErr
}

// tryAlternates offers one declined request to the engines after position
// from in the chain, in order, without moving the cursor. It reports the
// first answer and the engine that gave it. A further refusal moves on; any
// other failure moves on too, because an alternate that is offline is no
// reason to demote the preferred engine, which is still healthy.
func (r *Runner) tryAlternates(ctx context.Context, cs *chainState, from int, req engine.Request) (engine.Result, engine.Engine, bool) {
	declined := cs.engines[from]
	for j := from + 1; j < len(cs.engines); j++ {
		if ctx.Err() != nil {
			return engine.Result{}, nil, false
		}
		alt := cs.engines[j]
		r.warn(fmt.Sprintf("%s declined this prompt; trying %s for it", declined.Name(), alt.Name()))
		res, err := alt.Generate(ctx, req)
		if err == nil && contentRefusal(req, res) {
			err = fmt.Errorf("%s: %w", alt.Name(), engine.ErrRefused)
		}
		if err == nil {
			r.addUsage(res.Usage)
			return res, alt, true
		}
		r.warn(fmt.Sprintf("%s: %v", alt.Name(), err))
		declined = alt
	}
	return engine.Result{}, nil, false
}
