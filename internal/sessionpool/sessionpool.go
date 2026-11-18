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
// still alive and can be closed — so it is exercised in tests with a fake. The
// engine's ACP agent connection satisfies it directly and is pooled through it
// by the ACP engine.
//
// Lock order is entry before pool, never the reverse: Get holds an entry's
// lock across a slow start and then consults the pool map, so anything that
// held the pool lock while waiting for an entry would deadlock against it.
package sessionpool

import (
	"context"
	"sync"
	"time"
)

// Session is a reusable, closable backend process. draft's ACP agent
// connection satisfies it.
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
	for {
		e := p.entryFor(key)

		// Hold only this key's lock across the (possibly slow) start, so other
		// keys are unaffected and concurrent borrowers of this key share one
		// start.
		e.mu.Lock()
		// Reap or CloseAll may have dropped this entry between the lookup and
		// the lock. A session started on a dropped entry is in no map, so
		// nothing would ever reap or close it: look the key up again.
		if !p.current(key, e) {
			e.mu.Unlock()
			continue
		}
		sess, err := p.borrow(ctx, e, start)
		e.mu.Unlock()
		return sess, err
	}
}

// borrow marks e in use, first replacing its session if there is none or it
// has died. The caller holds e.mu.
func (p *Pool) borrow(ctx context.Context, e *entry, start StartFunc) (Session, error) {
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

// current reports whether e is still the map's entry for key.
func (p *Pool) current(key string, e *entry) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.entries[key] == e
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
//
// An entry whose lock is held is skipped rather than waited for: someone is
// borrowing it or starting its session, so it is not idle, and waiting would
// stall every Reap caller behind a start that can take minutes.
func (p *Pool) Reap() int {
	var doomed []Session
	for key, e := range p.snapshot() {
		if !e.mu.TryLock() {
			continue
		}
		if e.sess != nil && e.inUse == 0 && p.now().Sub(e.lastUsed) >= p.idle {
			doomed = append(doomed, e.sess)
			e.sess = nil
			p.drop(key, e)
		}
		e.mu.Unlock()
	}
	// Close outside every lock: closing an agent waits for its process to
	// exit, and nothing else should queue behind that.
	for _, s := range doomed {
		_ = s.Close()
	}
	return len(doomed)
}

// CloseAll closes every live session and empties the pool, regardless of idle
// time or in-use count. It is the end-of-queue shutdown: nothing should be
// borrowing when it runs, but it does not wait for borrowers.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	all := p.entries
	p.entries = map[string]*entry{}
	p.mu.Unlock()

	var doomed []Session
	for _, e := range all {
		e.mu.Lock()
		if e.sess != nil {
			doomed = append(doomed, e.sess)
			e.sess = nil
		}
		e.mu.Unlock()
	}
	for _, s := range doomed {
		_ = s.Close()
	}
}

// Len reports how many keys currently hold a live session.
func (p *Pool) Len() int {
	n := 0
	for _, e := range p.snapshot() {
		e.mu.Lock()
		if e.sess != nil {
			n++
		}
		e.mu.Unlock()
	}
	return n
}

// snapshot copies the entry map, so callers can visit entries without holding
// the pool lock while they take an entry's.
func (p *Pool) snapshot() map[string]*entry {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]*entry, len(p.entries))
	for k, e := range p.entries {
		out[k] = e
	}
	return out
}

// drop removes e from the map if it is still the entry for key. The caller
// holds e.mu, which is the permitted order.
func (p *Pool) drop(key string, e *entry) {
	p.mu.Lock()
	if p.entries[key] == e {
		delete(p.entries, key)
	}
	p.mu.Unlock()
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
