// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build unix

package engine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/internal/proc"
)

// These pin P-H1: a provider that leaves a background process holding its
// output must not hold the call open. Windows has no process groups and is
// best-effort, so they run on unix only.

// execWithPidfile is fakeExec with HELPER_PIDFILE set, so a test can find the
// grandchild the helper starts.
func execWithPidfile(mode, pidfile string, extra ...string) func(context.Context, string, ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmd := fakeExec(mode)(ctx, name, args...)
		cmd.Env = append(cmd.Env, append([]string{"HELPER_PIDFILE=" + pidfile}, extra...)...)
		return cmd
	}
}

// within runs fn and fails the test if it has not returned by limit, so a
// regression reports a hang instead of becoming one.
func within(t *testing.T, limit time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatalf("still blocked after %s", limit)
	}
}

// grandchildPid waits for the helper to record its grandchild's pid.
func grandchildPid(t *testing.T, pidfile string) int {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if b, err := os.ReadFile(pidfile); err == nil && len(b) > 0 {
			pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
			if err != nil {
				t.Fatal(err)
			}
			return pid
		}
	}
	t.Fatal("the helper never recorded its grandchild")
	return 0
}

// alive reports whether pid is still running, allowing it a moment to die.
func alive(pid int) bool {
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return false
		}
	}
	return true
}

func TestSessionTimeoutKillsTheWholeProcessGroup(t *testing.T) {
	pidfile := filepath.Join(t.TempDir(), "pid")
	orig := execCommand
	execCommand = execWithPidfile("grandchild-hang", pidfile)
	defer func() { execCommand = orig }()

	s, _ := NewSession("codex", config.Config{CallTimeout: 300 * time.Millisecond})
	var err error
	within(t, 3*time.Second, func() { _, err = s.Generate(context.Background(), Request{Prompt: "x"}) })
	if err == nil {
		t.Fatal("a call that timed out must fail")
	}
	if pid := grandchildPid(t, pidfile); alive(pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("grandchild %d outlived the call", pid)
	}
}

func TestSessionSucceedsWhenAGrandchildHoldsStderr(t *testing.T) {
	pidfile := filepath.Join(t.TempDir(), "pid")
	orig, origDelay := execCommand, proc.WaitDelay
	execCommand = execWithPidfile("grandchild-linger", pidfile)
	proc.WaitDelay = 200 * time.Millisecond
	defer func() { execCommand, proc.WaitDelay = orig, origDelay }()

	s, _ := NewSession("codex", config.Config{CallTimeout: time.Minute})
	var res Result
	var err error
	within(t, 5*time.Second, func() { res, err = s.Generate(context.Background(), Request{Prompt: "x"}) })
	_ = syscall.Kill(grandchildPid(t, pidfile), syscall.SIGKILL)
	if err != nil {
		t.Fatalf("the CLI exited cleanly with its output complete; got %v", err)
	}
	if res.Text != "# Title\n\nbody text." {
		t.Errorf("text = %q", res.Text)
	}
}

func TestACPCloseReturnsWhenAGrandchildHoldsItsOutput(t *testing.T) {
	pidfile := filepath.Join(t.TempDir(), "pid")
	orig := execCommand
	execCommand = execWithPidfile("acp-agent", pidfile, "HELPER_ACP_GRANDCHILD=1")
	defer func() { execCommand = orig }()

	c, err := startACP(context.Background(), Provider{Name: "fake", Bin: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	within(t, 3*time.Second, c.close)
	if pid := grandchildPid(t, pidfile); alive(pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("grandchild %d outlived the agent", pid)
	}
}

// A process that left the group survives the kill, so the output it holds is
// closed under the parser instead; the call still ends at its timeout.
func TestSessionTimeoutEndsTheCallWhenAGrandchildEscapesTheGroup(t *testing.T) {
	pidfile := filepath.Join(t.TempDir(), "pid")
	orig, origDelay := execCommand, proc.WaitDelay
	execCommand = execWithPidfile("grandchild-hang", pidfile, "HELPER_DETACH=1")
	proc.WaitDelay = 200 * time.Millisecond
	defer func() { execCommand, proc.WaitDelay = orig, origDelay }()

	s, _ := NewSession("codex", config.Config{CallTimeout: 300 * time.Millisecond})
	var err error
	within(t, 3*time.Second, func() { _, err = s.Generate(context.Background(), Request{Prompt: "x"}) })
	_ = syscall.Kill(grandchildPid(t, pidfile), syscall.SIGKILL)
	if err == nil {
		t.Fatal("a call that timed out must fail")
	}
}
