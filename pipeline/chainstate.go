// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"time"

	"github.com/sebastienrousseau/draft/engine"
)

// chainState is one ordered engine chain plus the cursor into it. Each request
// kind owns one, so extraction failing over to Ollama does not drag writing
// down with it — the two are separately configurable and separately sticky.
type chainState struct {
	engines []engine.Engine
	cur     int
	// demotedAt is when the cursor last advanced past a failing engine.
	demotedAt time.Time
}

// timeNow is a variable so the rehabilitation clock is testable.
var timeNow = time.Now

// rehabilitationDelay is how long a demoted engine stays demoted before the
// chain probes it again.
//
// Stickiness is right for a provider that is genuinely dead — not logged in,
// uninstalled — because retrying it costs a failed call per job. It is wrong
// for one that blipped: a single flaky moment on the first paper otherwise
// demotes a forty-paper queue to the local model for its entire life, and the
// cost of that is every remaining article written by a 4B model instead of
// the backend the user chose. A failed call is cheap and fails fast; a
// silently downgraded queue is expensive and invisible. Probing occasionally
// is the cheaper mistake.
const rehabilitationDelay = 15 * time.Minute

// rehabilitate returns the chain to its preferred engine once a demoted one
// has had long enough to recover, and reports whether it did.
func (c *chainState) rehabilitate(now time.Time) bool {
	if c == nil || c.cur == 0 || c.demotedAt.IsZero() || now.Sub(c.demotedAt) < rehabilitationDelay {
		return false
	}
	c.cur = 0
	c.demotedAt = time.Time{}
	return true
}

func (c *chainState) active() engine.Engine {
	if c == nil || c.cur >= len(c.engines) {
		return nil
	}
	return c.engines[c.cur]
}
