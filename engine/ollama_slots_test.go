// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// singleSlotServer behaves like Ollama with OLLAMA_NUM_PARALLEL=1: it works on
// one request at a time and sends headers only when it starts one.
func singleSlotServer(t *testing.T, work time.Duration) *httptest.Server {
	t.Helper()
	var slot sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slot.Lock()
		defer slot.Unlock()
		time.Sleep(work) // load and prompt evaluation, before the first byte
		_, _ = w.Write([]byte(`{"response":"ok","done":true}` + "\n"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// twoAtOnce runs two concurrent generations and returns their errors.
func twoAtOnce(o *Ollama) []error {
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = o.Generate(context.Background(), Request{Prompt: "p"})
		}(i)
	}
	wg.Wait()
	return errs
}

// A request sent beyond the server's slots waits in its queue with no
// headers, so the header timeout fails it whenever the request ahead runs
// longer than the timeout: the failure a second extraction worker hit
// against a single-slot server on an 8 GB machine. Holding the request in
// draft until a slot is free keeps the header wait about the server alone.
func TestOllamaHoldsRequestsUntilTheServerHasASlot(t *testing.T) {
	withEnsureStub(t)
	srv := singleSlotServer(t, 150*time.Millisecond)
	client := func() *http.Client {
		return &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 250 * time.Millisecond}}
	}

	unbounded := &Ollama{host: srv.URL, client: client(), extract: "m", write: "m", edit: "m"}
	failed := 0
	for _, err := range twoAtOnce(unbounded) {
		if err != nil {
			failed++
		}
	}
	if failed == 0 {
		t.Fatal("the fake server does not reproduce the queued header timeout; the test proves nothing")
	}

	bounded := &Ollama{host: srv.URL, client: client(), extract: "m", write: "m", edit: "m", slots: make(chan struct{}, 1)}
	for i, err := range twoAtOnce(bounded) {
		if err != nil {
			t.Errorf("request %d failed with one slot held per request: %v", i, err)
		}
	}
}

// A caller that gives up while waiting for a slot gets its context error, not
// a request sent after it stopped waiting.
func TestOllamaSlotWaitHonoursTheContext(t *testing.T) {
	o := &Ollama{slots: make(chan struct{}, 1)}
	o.slots <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := o.acquireSlot(ctx); err == nil {
		t.Error("acquireSlot returned a slot after its context was cancelled")
	}
}

func TestNewOllamaSizesItsSlotsFromTheEnvironment(t *testing.T) {
	t.Setenv("OLLAMA_NUM_PARALLEL", "3")
	if got := cap(NewOllama(configWithHost("http://127.0.0.1:1")).slots); got != 3 {
		t.Errorf("slots = %d, want 3", got)
	}
	t.Setenv("OLLAMA_NUM_PARALLEL", "")
	if got := cap(NewOllama(configWithHost("http://127.0.0.1:1")).slots); got != 1 {
		t.Errorf("slots = %d, want 1 by default", got)
	}
}
