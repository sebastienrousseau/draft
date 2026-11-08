// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build unix

package engine

import (
	"os/exec"
	"syscall"
)

// detach starts cmd in a session of its own, out of reach of a kill aimed at
// its parent's process group, as a daemonising helper would be.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
