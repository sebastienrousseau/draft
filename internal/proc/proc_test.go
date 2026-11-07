// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package proc

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestBoundSetsAWaitDelayAndACancel(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "true")
	Bound(cmd)
	if cmd.WaitDelay != WaitDelay {
		t.Errorf("WaitDelay = %s, want %s", cmd.WaitDelay, WaitDelay)
	}
	if cmd.Cancel == nil {
		t.Fatal("Bound must replace the default Cancel")
	}
	// Before Start there is nothing to kill, and cancelling must say so
	// quietly rather than panic on a nil Process.
	if err := cmd.Cancel(); err != nil {
		t.Errorf("Cancel before Start = %v", err)
	}
}

func TestBoundCommandIsKilledOnCancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX sleep")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sleep", "30")
	Bound(cmd)
	start := time.Now()
	if err := cmd.Run(); err == nil {
		t.Fatal("a cancelled command must fail")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("returned after %s", d)
	}
}
