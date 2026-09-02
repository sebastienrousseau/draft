// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/config"
	"github.com/sebastienrousseau/draft/engine"
)

// healthyProbes reports a machine with everything installed.
func healthyProbes() probes {
	return probes{
		lookPath:   func(name string) (string, error) { return "/usr/bin/" + name, nil },
		ollamaUp:   func(string) bool { return true },
		writable:   func(string) error { return nil },
		goos:       "darwin",
		providerOK: func(string) bool { return true },
	}
}

func doctorCfg(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	return config.Config{
		DraftsDir:  filepath.Join(dir, "drafts"),
		SourcesDir: filepath.Join(dir, "sources"),
		CacheDir:   filepath.Join(dir, "cache"),
		OllamaHost: config.OllamaHost,
		Engine:     config.EngineAuto,
	}
}

func TestDoctorReportsAReadyMachine(t *testing.T) {
	var out strings.Builder
	if code := runDoctor(doctorCfg(t), healthyProbes(), &out); code != 0 {
		t.Fatalf("exit code %d, output:\n%s", code, out.String())
	}
	for _, want := range []string{"SOURCE TOOLING", "BACKENDS", "PATHS", "ROUTING", "Ready."} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report missing %q:\n%s", want, out.String())
		}
	}
}

// Poppler is the one hard requirement for PDFs, and it was previously
// discovered at failure time — after the user had committed to a run.
func TestDoctorFailsWithoutPdftotext(t *testing.T) {
	p := healthyProbes()
	p.lookPath = func(name string) (string, error) {
		if name == "pdftotext" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + name, nil
	}
	var out strings.Builder
	if code := runDoctor(doctorCfg(t), p, &out); code != 1 {
		t.Fatalf("exit code %d, want 1:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "Poppler") {
		t.Errorf("report should name the remedy:\n%s", out.String())
	}
}

func TestDoctorFailsWithNoBackendAtAll(t *testing.T) {
	p := healthyProbes()
	p.providerOK = func(string) bool { return false }
	p.lookPath = func(name string) (string, error) {
		if name == "ollama" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + name, nil
	}
	var out strings.Builder
	if code := runDoctor(doctorCfg(t), p, &out); code != 1 {
		t.Fatalf("exit code %d, want 1:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "Install an agent CLI") {
		t.Errorf("report should name the remedy:\n%s", out.String())
	}
}

func TestDoctorFailsWithNothingInstalled(t *testing.T) {
	p := healthyProbes()
	p.providerOK = func(string) bool { return false }
	p.lookPath = func(string) (string, error) { return "", errors.New("not found") }
	var out strings.Builder
	if code := runDoctor(doctorCfg(t), p, &out); code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}
	if !strings.Contains(out.String(), "No source tooling and no backend") {
		t.Errorf("report should say both are missing:\n%s", out.String())
	}
}

// The drafts directory is the one place a run must not discover a problem
// after spending ten minutes on a model.
func TestDoctorFailsOnAnUnwritableDraftsDirectory(t *testing.T) {
	p := healthyProbes()
	p.writable = func(string) error { return errors.New("permission denied") }
	var out strings.Builder
	if code := runDoctor(doctorCfg(t), p, &out); code != 1 {
		t.Fatalf("exit code %d, want 1:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "not writable") {
		t.Errorf("report should name the problem:\n%s", out.String())
	}
}

func TestDoctorNotesOptionalGaps(t *testing.T) {
	cfg := doctorCfg(t)
	cfg.CacheDir = "" // caching disabled
	cfg.StrictNumbers = true
	p := healthyProbes()
	p.goos = "linux" // no textutil
	p.ollamaUp = func(string) bool { return false }
	p.providerOK = func(name string) bool { return name == "claude" }

	var out strings.Builder
	if code := runDoctor(cfg, p, &out); code != 0 {
		t.Fatalf("optional gaps should not fail: exit %d\n%s", code, out.String())
	}
	for _, want := range []string{
		"macOS only", "disabled; every run re-extracts", "strict numbers", "not responding",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report missing %q:\n%s", want, out.String())
		}
	}
}

func TestDoctorNotesNoSessionProviders(t *testing.T) {
	p := healthyProbes()
	p.providerOK = func(string) bool { return false }
	var out strings.Builder
	if code := runDoctor(doctorCfg(t), p, &out); code != 0 {
		t.Fatalf("Ollama alone is a usable machine: exit %d", code)
	}
	if !strings.Contains(out.String(), "none installed") {
		t.Errorf("report should say so:\n%s", out.String())
	}
}

func TestDoctorNotesAnUnwritableCache(t *testing.T) {
	cfg := doctorCfg(t)
	p := healthyProbes()
	p.writable = func(dir string) error {
		if dir == cfg.CacheDir {
			return errors.New("read-only")
		}
		return nil
	}
	var out strings.Builder
	if code := runDoctor(cfg, p, &out); code != 0 {
		t.Fatalf("an unwritable cache is not fatal: exit %d", code)
	}
	if !strings.Contains(out.String(), "runs will not be cached") {
		t.Errorf("report should say so:\n%s", out.String())
	}
}

func TestDoctorReportsExperimentalProviders(t *testing.T) {
	t.Cleanup(engine.ResetProviders)
	p := healthyProbes()
	var out strings.Builder
	runDoctor(doctorCfg(t), p, &out)
	if !strings.Contains(out.String(), "experimental") {
		t.Errorf("report should mark experimental providers:\n%s", out.String())
	}
}

func TestDirWritable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deeper")
	if err := dirWritable(dir); err != nil {
		t.Fatalf("dirWritable() = %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("dirWritable left %d file(s) behind", len(entries))
	}

	// A path occupied by a file cannot be a directory.
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dirWritable(file); err == nil {
		t.Error("dirWritable() on a file = nil, want an error")
	}
}

func TestDefaultProbesArePopulated(t *testing.T) {
	p := defaultProbes()
	if p.lookPath == nil || p.ollamaUp == nil || p.writable == nil || p.providerOK == nil || p.goos == "" {
		t.Error("defaultProbes left a field unset")
	}
}

func TestRunDoctorFlag(t *testing.T) {
	t.Setenv("DRAFT_DRAFTS_DIR", t.TempDir())
	t.Setenv("DRAFT_SOURCES_DIR", t.TempDir())
	var out, errb strings.Builder
	// The exit code depends on this machine; the report must render either way.
	run([]string{"--doctor"}, &out, &errb)
	if !strings.Contains(out.String(), "BACKENDS") {
		t.Errorf("--doctor produced no report:\n%s%s", out.String(), errb.String())
	}
}
