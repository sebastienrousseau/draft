// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"fmt"
	"time"
)

// eta estimates how much of a multi-section extraction is left.
//
// The estimate is a running mean of completed sections rather than a
// projection from the first one: section 0 settles the engine chain and warms
// the model, so it is systematically slower than the rest and extrapolating
// from it overstates the remaining time badly on a long paper.
type eta struct {
	total int
	// seed is the first section's duration, used only until a second
	// observation lands — a rough number beats no number at all.
	seed  time.Duration
	sum   time.Duration
	count int
}

func newETA(total int, seed time.Duration) *eta {
	return &eta{total: total, seed: seed}
}

// observe records how long one section took.
func (e *eta) observe(d time.Duration) {
	e.sum += d
	e.count++
}

// remaining renders the estimate for the sections after done, as a suffix
// ready to append to a progress line. It returns "" when there is nothing
// useful to say — no observations yet, or nothing left to do.
func (e *eta) remaining(done int) string {
	if e == nil || done >= e.total {
		return ""
	}
	per := e.seed
	if e.count > 0 {
		per = e.sum / time.Duration(e.count)
	}
	if per <= 0 {
		return ""
	}
	left := time.Duration(e.total-done) * per
	if left < time.Second {
		return ""
	}
	return " · ~" + humanDuration(left) + " remaining"
}

// humanDuration renders a coarse duration: minutes matter to someone deciding
// whether to wait, tenths of a second do not.
func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}
