// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build unix

package pdf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A background process the tool leaves behind inherits its output. It must
// not hold runTool open past the timeout, nor outlive it (P-H1).
func TestRunToolTimeoutKillsTheWholeProcessGroup(t *testing.T) {
	pidfile := filepath.Join(t.TempDir(), "pid")
	start := time.Now()
	_, err := runTool(context.Background(), 300*time.Millisecond, "sh", "-c", `sleep 30 & echo $! > "$0"; wait`, pidfile)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected a timeout error, got %v", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("returned after %s: a grandchild held the call open", d)
	}
	b, err := os.ReadFile(pidfile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Errorf("grandchild %d outlived the call", pid)
}
