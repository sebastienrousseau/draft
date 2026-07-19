// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pdf

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunToolSuccess(t *testing.T) {
	out, err := runTool(context.Background(), time.Second, "echo", "hello")
	if err != nil || !strings.Contains(out, "hello") {
		t.Errorf("runTool echo failed: %q %v", out, err)
	}
}

func TestRunToolMissingBinary(t *testing.T) {
	if _, err := runTool(context.Background(), time.Second, "definitely-not-a-real-binary-xyz"); err == nil {
		t.Error("missing binary should error")
	}
}

func TestRunToolNonZeroExit(t *testing.T) {
	if _, err := runTool(context.Background(), time.Second, "false"); err == nil {
		t.Error("non-zero exit should error")
	}
}

func TestExtractPDFInvalid(t *testing.T) {
	// Not a real PDF -> pdftotext exits non-zero -> error from the .pdf branch.
	p := filepath.Join(t.TempDir(), "fake.pdf")
	os.WriteFile(p, []byte("not a pdf"), 0o644)
	if _, err := Extract(context.Background(), p); err == nil || !strings.Contains(err.Error(), "pdftotext") {
		t.Errorf("expected pdftotext error, got %v", err)
	}
}

func TestExtractDOCXInvalid(t *testing.T) {
	p := filepath.Join(t.TempDir(), "fake.docx")
	os.WriteFile(p, []byte("not a docx"), 0o644)
	// textutil either errors or returns empty; either way the .docx branch runs.
	_, _ = Extract(context.Background(), p)
}
