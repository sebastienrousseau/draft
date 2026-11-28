// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"context"
	"os"
	"strconv"
	"strings"
)

// OllamaParallelism is how many generations draft keeps in flight against
// one Ollama server: OLLAMA_NUM_PARALLEL from draft's environment when it is
// a positive number, and one otherwise.
//
// One is the default because draft cannot see the server's setting (a server
// started by launchd or another shell has its own environment), and sending
// more than the server runs is not harmless queueing. Ollama sends no
// response headers until it starts a request, so a request queued behind a
// long one sits out the header timeout: on an 8 GB machine with
// OLLAMA_NUM_PARALLEL=1, a second extraction worker failed with "timeout
// awaiting response headers" while the first spent 6 min 38 s on its section.
// Export OLLAMA_NUM_PARALLEL to draft as well as the server to run more.
func OllamaParallelism() int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("OLLAMA_NUM_PARALLEL"))); err == nil && v > 0 {
		return v
	}
	return 1
}

// acquireSlot waits for one of the engine's in-flight slots, so a request is
// sent only when the server can start it and the header timeout measures the
// server, not a queue. It gives up when ctx does. An engine built without
// slots (a test literal) does not limit.
func (o *Ollama) acquireSlot(ctx context.Context) (release func(), err error) {
	if o.slots == nil {
		return func() {}, nil
	}
	select {
	case o.slots <- struct{}{}:
		return func() { <-o.slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
