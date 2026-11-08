// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !unix

package proc

import "os/exec"

// isolate does nothing here: without process groups, only the process itself
// is killed, and WaitDelay alone keeps a lingering child from holding the
// call open. Windows is best-effort until a Job Object is wired in.
func isolate(*exec.Cmd) {}

func kill(cmd *exec.Cmd) error { return cmd.Process.Kill() }
