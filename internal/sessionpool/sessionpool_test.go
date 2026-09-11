// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package sessionpool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSession is a Session whose liveness is controllable and whose closes are
// counted, so a test can drive the pool's lifecycle without a real process.
type fakeSession struct {
	alive  atomic.Bool
	closes atomic.Int64
}

func newFake() *fakeSession {
	f := &fakeSession{}
	f.alive.Store(true)
	return f
}

func (f *fakeSession) Alive() bool  { return f.alive.Load() }
func (f *fakeSession) Close() error { f.closes.Add(1); f.alive.Store(false); return nil }

// starter returns a StartFunc that hands out fresh fakes and counts its calls.
func starter() (StartFunc, *atomic.Int64, *[]*fakeSession) {
	var n atomic.Int64
	var mu sync.Mutex
	var made []*fakeSession
	fn := func(context.Context) (Session, error) {
		n.Add(1)
		f := newFake()
		mu.Lock()
		made = append(made, f)
		mu.Unlock()
		return f, nil
	}
	return fn, &n, &made
}

// testClock is a race-safe controllable clock.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func TestGetStartsOnceAndReuses(t *testing.T) {
	start, n, made := starter()
	p := New(time.Minute)
	ctx := context.Background()

	first, err := p.Get(ctx, "a", start)
	if err != nil {
		t.Fatal(err)
	}
	p.Release("a")
	second, err := p.Get(ctx, "a", start)
	if err != nil {
		t.Fatal(err)
	}
	p.Release("a")

	if n.Load() != 1 {
		t.Errorf("start called %d times, want 1 (session should be reused)", n.Load())
	}
	if first != second {
		t.Error("Get returned a different session for the same live key")
	}
	if got := (*made)[0].closes.Load(); got != 0 {
		t.Errorf("a live reused session was closed %d times, want 0", got)
	}
}

func TestGetRestartsDeadSession(t *testing.T) {
	start, n, made := starter()
	p := New(time.Minute)
	ctx := context.Background()

	if _, err := p.Get(ctx, "a", start); err != nil {
		t.Fatal(err)
	}
	p.Release("a")
	(*made)[0].alive.Store(false) // the process exited between borrows

	if _, err := p.Get(ctx, "a", start); err != nil {
		t.Fatal(err)
	}
	p.Release("a")

	if n.Load() != 2 {
		t.Errorf("start called %d times, want 2 (dead session must be replaced)", n.Load())
	}
	if got := (*made)[0].closes.Load(); got != 1 {
		t.Errorf("the dead session was closed %d times, want 1", got)
	}
}

func TestKeyedIsolation(t *testing.T) {
	start, n, _ := starter()
	p := New(time.Minute)
	ctx := context.Background()

	a, _ := p.Get(ctx, "a", start)
	b, _ := p.Get(ctx, "b", start)
	if a == b {
		t.Error("different keys share a session")
	}
	if n.Load() != 2 {
		t.Errorf("start called %d times for two keys, want 2", n.Load())
	}
	if p.Len() != 2 {
		t.Errorf("Len = %d, want 2", p.Len())
	}
}

func TestReapClosesOnlyIdleReleasedSessions(t *testing.T) {
	start, _, made := starter()
	clk := &testClock{t: time.Unix(1_000_000, 0)}
	p := New(10*time.Second, WithClock(clk.now))
	ctx := context.Background()

	if _, err := p.Get(ctx, "a", start); err != nil {
		t.Fatal(err)
	}
	p.Release("a")

	clk.advance(9 * time.Second)
	if r := p.Reap(); r != 0 {
		t.Errorf("reaped %d before idle elapsed, want 0", r)
	}
	if p.Len() != 1 {
		t.Errorf("Len = %d after early reap, want 1", p.Len())
	}

	clk.advance(2 * time.Second) // now 11s idle, past the 10s window
	if r := p.Reap(); r != 1 {
		t.Errorf("reaped %d after idle elapsed, want 1", r)
	}
	if p.Len() != 0 {
		t.Errorf("Len = %d after reap, want 0", p.Len())
	}
	if got := (*made)[0].closes.Load(); got != 1 {
		t.Errorf("reaped session closed %d times, want 1", got)
	}
}

func TestReapSkipsInUse(t *testing.T) {
	start, _, _ := starter()
	clk := &testClock{t: time.Unix(2_000_000, 0)}
	p := New(0, WithClock(clk.now)) // idle 0: eligible as soon as released
	ctx := context.Background()

	if _, err := p.Get(ctx, "a", start); err != nil {
		t.Fatal(err)
	}
	clk.advance(time.Hour)
	if r := p.Reap(); r != 0 {
		t.Errorf("reaped an in-use session (%d), want 0", r)
	}
	p.Release("a")
	if r := p.Reap(); r != 1 {
		t.Errorf("reaped %d after release, want 1", r)
	}
}

func TestReleaseUnknownAndUnderflow(t *testing.T) {
	start, _, _ := starter()
	p := New(0)
	ctx := context.Background()

	p.Release("never-borrowed") // must not panic

	if _, err := p.Get(ctx, "a", start); err != nil {
		t.Fatal(err)
	}
	p.Release("a")
	p.Release("a") // extra release must not drive inUse negative

	// With inUse pinned at 0, the session is still reapable exactly once.
	if r := p.Reap(); r != 1 {
		t.Errorf("reaped %d, want 1 (underflow guard should keep it reapable)", r)
	}
}

func TestCloseAll(t *testing.T) {
	start, _, made := starter()
	p := New(time.Hour)
	ctx := context.Background()

	// Borrow two keys and do not release them: CloseAll ignores in-use count.
	if _, err := p.Get(ctx, "a", start); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Get(ctx, "b", start); err != nil {
		t.Fatal(err)
	}
	p.CloseAll()
	if p.Len() != 0 {
		t.Errorf("Len = %d after CloseAll, want 0", p.Len())
	}
	for i, f := range *made {
		if f.closes.Load() != 1 {
			t.Errorf("session %d closed %d times, want 1", i, f.closes.Load())
		}
	}
}

func TestStartErrorLeavesNoSession(t *testing.T) {
	p := New(time.Minute)
	ctx := context.Background()
	boom := errors.New("cannot start agent")

	if _, err := p.Get(ctx, "a", func(context.Context) (Session, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("Get error = %v, want %v", err, boom)
	}
	if p.Len() != 0 {
		t.Errorf("Len = %d after failed start, want 0", p.Len())
	}
	// A later good start on the same key must succeed (the key is not poisoned).
	if _, err := p.Get(ctx, "a", func(context.Context) (Session, error) { return newFake(), nil }); err != nil {
		t.Errorf("retry after failed start errored: %v", err)
	}
}

// Concurrent borrowers of one live key must share a single start, with no data
// race. Run the suite with -race for the guarantee.
func TestConcurrentBorrowSharesOneStart(t *testing.T) {
	start, n, _ := starter()
	p := New(time.Hour)
	ctx := context.Background()

	const goroutines, iters = 64, 200
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				if _, err := p.Get(ctx, "shared", start); err != nil {
					t.Errorf("Get: %v", err)
					return
				}
				p.Release("shared")
			}
		}()
	}
	wg.Wait()

	if n.Load() != 1 {
		t.Errorf("start called %d times under contention, want 1", n.Load())
	}
	if p.Len() != 1 {
		t.Errorf("Len = %d, want 1", p.Len())
	}
}

// Get/Release racing against a reaping goroutine must not deadlock, panic, or
// race; the pool must be empty after shutdown. Exercises the pool↔entry lock
// ordering under contention.
func TestConcurrentGetReleaseReap(t *testing.T) {
	start, _, _ := starter()
	p := New(0) // idle 0: the reaper closes released sessions aggressively
	ctx := context.Background()

	stop := make(chan struct{})
	var reaper sync.WaitGroup
	reaper.Add(1)
	go func() {
		defer reaper.Done()
		for {
			select {
			case <-stop:
				return
			default:
				p.Reap()
			}
		}
	}()

	var wg sync.WaitGroup
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := []string{"a", "b", "c"}[id%3]
			for i := 0; i < 100; i++ {
				if _, err := p.Get(ctx, key, start); err != nil {
					t.Errorf("Get: %v", err)
					return
				}
				p.Release(key)
			}
		}(g)
	}
	wg.Wait()
	close(stop)
	reaper.Wait()

	p.CloseAll()
	if p.Len() != 0 {
		t.Errorf("Len = %d after shutdown, want 0", p.Len())
	}
}
