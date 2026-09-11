// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package sessionpool keeps one live backend process per key, reused across
// jobs and closed once it has been idle.
//
// A provider CLI's startup floor is seconds — measured at ~8.5s warm and up to
// ~128s cold on the slowest agent — and a batch queue that starts a fresh
// process per source pays it every time. Reusing one process across the whole
// queue pays it once. The pool is keyed (by provider name, in draft's use) so
// two request kinds routed to the same provider share a process, while two
// different providers each keep their own; and it closes a process that has
// gone unused for the idle interval so a stalled or finished queue does not
// hold a subprocess open forever.
//
// The pool is generic over a Session — anything that can report whether it is
// still alive and can be closed — so it is exercised in tests with a fake, and
// the engine's ACP agent connection plugs in behind a small adapter. Wiring it
// into the ACP engine, and validating the lifecycle against a live agent, is a
// separate step; this package is the concurrency core, complete and tested on
// its own.
package sessionpool

import (
	"context"
	"sync"
	"time"
)

// Session is a reusable, closable backend process. draft's ACP agent
// connection satisfies it behind an adapter (its alive/close become
// Alive/Close).
type Session interface {
	// Alive reports whether the session is still usable. A process that has
	// exited returns false, and the pool starts a replacement.
	Alive() bool
	// Close shuts the session down. The pool calls it once per session, when
	// the session is reaped or the pool is closed.
	Close() error
}

// StartFunc opens a new session for a key. The pool calls it when a key has no
// live session; concurrent first borrowers of the same key wait on one call
// rather than each starting a process.
type StartFunc func(ctx context.Context) (Session, error)

// Pool is a set of keyed, reused sessions with an idle close policy. The zero
// value is not usable; construct one with New. A Pool is safe for concurrent
// use.
type Pool struct {
	idle time.Duration
	now  func() time.Time

	mu      sync.Mutex
	entries map[string]*entry
}

// entry holds one key's session and its borrow state. Its own mutex guards a
// possibly-slow start without holding the pool lock, so starting a session for
// one key never blocks borrows of another.
type entry struct {
	mu       sync.Mutex
	sess     Session
	inUse    int
	lastUsed time.Time
}

// Option configures a Pool.
type Option func(*Pool)

// WithClock overrides the clock the pool reads for idle decisions. It exists
// for deterministic tests; production uses time.Now.
func WithClock(now func() time.Time) Option {
	return func(p *Pool) { p.now = now }
}

// New returns a pool that closes a session after it has been idle for at least
// idle. An idle of zero means a released session is eligible to be reaped
// immediately.
func New(idle time.Duration, opts ...Option) *Pool {
	p := &Pool{idle: idle, now: time.Now, entries: map[string]*entry{}}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Get returns a live session for key, starting one with start if the key has
// none or its session has died. The returned session is marked in use until a
// matching Release, so it cannot be reaped while a caller holds it. A start
// error is returned and leaves the key with no live session.
func (p *Pool) Get(ctx context.Context, key string, start StartFunc) (Session, error) {
	e := p.entryFor(key)

	// Hold only this key's lock across the (possibly slow) start, so other
	// keys are unaffected and concurrent borrowers of this key share one start.
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sess == nil || !e.sess.Alive() {
		if e.sess != nil {
			_ = e.sess.Close()
			e.sess = nil
		}
		sess, err := start(ctx)
		if err != nil {
			return nil, err
		}
		e.sess = sess
	}
	e.inUse++
	e.lastUsed = p.now()
	return e.sess, nil
}

// Release returns a session borrowed with Get. After the last borrower
// releases it, the session becomes eligible for reaping once idle. Releasing a
// key with no live borrow is a no-op, so a defer is always safe.
func (p *Pool) Release(key string) {
	p.mu.Lock()
	e := p.entries[key]
	p.mu.Unlock()
	if e == nil {
		return
	}
	e.mu.Lock()
	if e.inUse > 0 {
		e.inUse--
	}
	e.lastUsed = p.now()
	e.mu.Unlock()
}

// Reap closes and drops every session that is not in use and has been idle for
// at least the pool's idle interval. It returns how many it closed. Call it
// periodically (a ticker) to bound how long an unused process lingers.
func (p *Pool) Reap() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	reaped := 0
	for key, e := range p.entries {
		e.mu.Lock()
		if e.sess != nil && e.inUse == 0 && p.now().Sub(e.lastUsed) >= p.idle {
			_ = e.sess.Close()
			e.sess = nil
			delete(p.entries, key)
			reaped++
		}
		e.mu.Unlock()
	}
	return reaped
}

// CloseAll closes every live session and empties the pool, regardless of idle
// time or in-use count. It is the end-of-queue shutdown: nothing should be
// borrowing when it runs, but it does not wait for borrowers.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, e := range p.entries {
		e.mu.Lock()
		if e.sess != nil {
			_ = e.sess.Close()
			e.sess = nil
		}
		e.mu.Unlock()
		delete(p.entries, key)
	}
}

// Len reports how many keys currently hold a live session.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, e := range p.entries {
		e.mu.Lock()
		if e.sess != nil {
			n++
		}
		e.mu.Unlock()
	}
	return n
}

// entryFor returns the entry for key, creating an empty one under the pool
// lock. The pool lock is held only for the map lookup, never across a start.
func (p *Pool) entryFor(key string) *entry {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.entries[key]
	if e == nil {
		e = &entry{}
		p.entries[key] = e
	}
	return e
}
