// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"strings"
	"testing"
	"time"
)

func TestETAUsesARunningMeanNotTheFirstSection(t *testing.T) {
	// Section 0 settles the engine chain and warms the model, so it is
	// systematically slower. Extrapolating from it overstates the remaining
	// time badly on a long paper.
	e := newETA(11, 60*time.Second)

	// Before any observation, the seed is all there is.
	if got := e.remaining(1); !strings.Contains(got, "10m") {
		t.Errorf("seeded estimate = %q, want ~10 minutes", got)
	}

	// Real sections come in at 6s each.
	for i := 0; i < 4; i++ {
		e.observe(6 * time.Second)
	}
	got := e.remaining(5)
	if !strings.Contains(got, "36s") {
		t.Errorf("estimate = %q, want ~36s (6 sections x 6s)", got)
	}
}

func TestETAIsSilentWhenItHasNothingUseful(t *testing.T) {
	for _, tc := range []struct {
		name string
		eta  *eta
		done int
	}{
		{name: "nil", eta: nil, done: 1},
		{name: "finished", eta: newETA(3, time.Minute), done: 3},
		{name: "past the end", eta: newETA(3, time.Minute), done: 9},
		{name: "no timing at all", eta: newETA(10, 0), done: 1},
		{name: "sub-second remainder", eta: newETA(2, time.Millisecond), done: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.eta.remaining(tc.done); got != "" {
				t.Errorf("remaining = %q, want empty", got)
			}
		})
	}
}

func TestHumanDuration(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{45 * time.Second, "45s"},
		{90 * time.Second, "1m30s"},
		{10 * time.Minute, "10m00s"},
		{75 * time.Minute, "1h15m"},
	} {
		if got := humanDuration(tc.d); got != tc.want {
			t.Errorf("humanDuration(%s) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
