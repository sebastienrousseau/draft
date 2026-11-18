// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build unix

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Opening a FIFO for reading blocks until something writes to it, so a
// manifest naming one hung --verify for ever.
func TestVerifyDoesNotHangOnAFIFOSource(t *testing.T) {
	day, stem, sourcePath := writeRichSet(t, "the source text", false, false)
	fifo := filepath.Join(t.TempDir(), "paper.pdf")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("cannot make a FIFO here: %v", err)
	}
	pointSourceAt(t, day, stem, sourcePath, fifo)

	done := make(chan error, 1)
	go func() {
		_, _, _, err := loadSet(filepath.Join(day, "final", stem+"-final.md"))
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("a FIFO named as a source was not refused: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("--verify hung opening a FIFO named as a source")
	}
}

// A source the verifier may not read is as good as absent here: nothing
// about it can be confirmed, and nothing about it is wrong either.
func TestDigestSourceTreatsAnUnreadableFileAsAbsent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads any file")
	}
	path := filepath.Join(t.TempDir(), "paper.pdf")
	if err := os.WriteFile(path, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	if sum, found, err := digestSource(path); found || sum != "" || err != nil {
		t.Errorf("digestSource(unreadable) = %q, %v, %v", sum, found, err)
	}
}
