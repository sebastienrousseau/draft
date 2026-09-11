// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"context"
	"errors"
	"testing"
)

// fakeEntailer supports the claims named in `yes`, and returns err (if set)
// for every claim, so both the verdict and the failure paths are exercised.
type fakeEntailer struct {
	yes map[string]bool
	err error
}

func (f fakeEntailer) Supports(_ context.Context, claim, _ string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.yes[claim], nil
}

func recs(claims ...string) []Record {
	out := make([]Record, len(claims))
	for i, c := range claims {
		out[i] = Record{Claim: c, SourceQuote: "quote for " + c}
	}
	return out
}

func TestSecondPassDropsUnsupported(t *testing.T) {
	in := recs("kept-1", "dropped", "kept-2")
	e := fakeEntailer{yes: map[string]bool{"kept-1": true, "kept-2": true}} // "dropped" absent -> unsupported
	kept, dropped := SecondPass(context.Background(), in, e)
	if dropped != 1 || len(kept) != 2 {
		t.Fatalf("kept %d, dropped %d; want 2, 1", len(kept), dropped)
	}
	for _, r := range kept {
		if r.Claim == "dropped" {
			t.Error("unsupported claim survived")
		}
	}
}

func TestSecondPassFailOpenOnError(t *testing.T) {
	in := recs("a", "b")
	e := fakeEntailer{err: errors.New("model offline")}
	kept, dropped := SecondPass(context.Background(), in, e)
	if dropped != 0 || len(kept) != 2 {
		t.Errorf("kept %d, dropped %d; want 2, 0 (fail-open must keep every claim)", len(kept), dropped)
	}
}

func TestSecondPassCancellationKeepsRemaining(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	in := recs("a", "b", "c")
	// Even an entailer that would drop everything must not: the pass stops on a
	// cancelled context and keeps what is left.
	e := fakeEntailer{yes: map[string]bool{}}
	kept, dropped := SecondPass(ctx, in, e)
	if dropped != 0 || len(kept) != 3 {
		t.Errorf("kept %d, dropped %d; want 3, 0 on cancellation", len(kept), dropped)
	}
}

func TestSecondPassEmpty(t *testing.T) {
	kept, dropped := SecondPass(context.Background(), nil, fakeEntailer{})
	if len(kept) != 0 || dropped != 0 {
		t.Errorf("empty input = %d kept, %d dropped; want 0, 0", len(kept), dropped)
	}
}
