// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package proc bounds the lifetime of the external processes draft starts.
//
// Killing a process on timeout is not enough to end a call. A provider CLI
// can leave a background process behind, and that process inherits the
// CLI's stdout and stderr: Wait then waits for output that never closes, and
// a three-second timeout was seen still blocked after three minutes. So a
// bounded command runs in its own process group, cancellation kills the
// whole group, and WaitDelay caps how long Wait lingers for output once the
// process is gone.
package proc

import (
	"os/exec"
	"time"
)

// WaitDelay is how long Wait waits for a process's output to close once the
// process has exited or its context is done. A variable so tests can shorten it.
var WaitDelay = 5 * time.Second

// Bound sets cmd up so that neither the process nor anything it starts can
// hold a call open past its context. Call it before Start.
func Bound(cmd *exec.Cmd) {
	cmd.WaitDelay = WaitDelay
	isolate(cmd)
	cmd.Cancel = func() error { return Kill(cmd) }
}

// Kill kills cmd's process and, where the platform allows, everything it
// started. It is a no-op for a command that never started.
func Kill(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return kill(cmd)
}
