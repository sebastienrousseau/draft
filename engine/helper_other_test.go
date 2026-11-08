// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !unix

package engine

import "os/exec"

// detach has nothing to do without process groups to leave.
func detach(*exec.Cmd) {}
