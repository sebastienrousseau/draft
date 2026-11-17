// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Port 0 asks the OS for any free port, so a server started for it binds
// somewhere the client will never look. Starting one can only leak it.
func TestEnsureOllamaRunningRefusesPortZero(t *testing.T) {
	started := 0
	orig := startCommand
	startCommand = func(name string, args ...string) *exec.Cmd {
		started++
		return exec.Command("true")
	}
	defer func() { startCommand = orig }()
	withAvailable(map[string]bool{"ollama": true}, func() {
		err := EnsureOllamaRunning("http://127.0.0.1:0")
		if err == nil || !strings.Contains(err.Error(), "port 0") {
			t.Errorf("err = %v, want a port 0 refusal", err)
		}
	})
	if started != 0 {
		t.Errorf("started %d server(s) for an unreachable host", started)
	}
}

// A server draft started that never answers is draft's to stop: returning
// an error while it keeps running leaves an orphan behind every run.
func TestEnsureOllamaRunningStopsAServerThatNeverAnswers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX sleep process")
	}
	var cmd *exec.Cmd
	orig, origWait := startCommand, ollamaStartWait
	startCommand = func(name string, args ...string) *exec.Cmd {
		cmd = exec.Command("sleep", "30")
		return cmd
	}
	ollamaStartWait = 300 * time.Millisecond
	defer func() { startCommand, ollamaStartWait = orig, origWait }()

	withAvailable(map[string]bool{"ollama": true}, func() {
		if err := EnsureOllamaRunning("http://127.0.0.1:1"); err == nil {
			t.Fatal("an unresponsive server must be reported")
		}
	})
	if cmd == nil || cmd.Process == nil {
		t.Fatal("the server was never started")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := cmd.Process.Signal(syscall.Signal(0)); errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	t.Fatalf("pid %d is still running after EnsureOllamaRunning gave up", cmd.Process.Pid)
}
