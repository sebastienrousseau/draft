// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveHomeFallsBackToTemp(t *testing.T) {
	origHome, origWD, origTemp := userHomeDir, getwd, tempDir
	defer func() { userHomeDir, getwd, tempDir = origHome, origWD, origTemp }()
	userHomeDir = func() (string, error) { return "", errors.New("no home") }
	getwd = func() (string, error) { return "", errors.New("no cwd") }
	tempDir = func() string { return "/fallback-temp" }
	var warnings []string
	got := resolveHome(func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	})
	if got != "/fallback-temp" || len(warnings) != 1 {
		t.Fatalf("temp fallback = %q, warnings %v", got, warnings)
	}
}

// Load used to discard the os.UserHomeDir error, leaving HomeDir empty and
// turning SourcesDir and DraftsDir into relative paths — so drafts were
// written into whatever directory the process started in. Both must now be
// absolute whatever the environment, and the fallback must be reported.
func TestLoadHomeUnavailableStillYieldsAbsolutePaths(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "") // Windows equivalent

	c := Load(Flags{})

	if !filepath.IsAbs(c.DraftsDir) {
		t.Errorf("DraftsDir %q is not absolute", c.DraftsDir)
	}
	if !filepath.IsAbs(c.SourcesDir) {
		t.Errorf("SourcesDir %q is not absolute", c.SourcesDir)
	}
	if !filepath.IsAbs(c.HomeDir) {
		t.Errorf("HomeDir %q is not absolute", c.HomeDir)
	}
	if !hasWarning(c, "home directory") {
		t.Errorf("expected a warning about the home directory, got %v", c.Warnings)
	}
}

func TestLoadHomeAvailableWarnsNothing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if c := Load(Flags{}); len(c.Warnings) != 0 {
		t.Errorf("expected no warnings, got %v", c.Warnings)
	}
}

func TestResolveOllamaHost(t *testing.T) {
	for _, tc := range []struct {
		name    string
		env     string
		want    string
		warning string
	}{
		{name: "default is untouched", env: "", want: OllamaHost},
		{name: "explicit default", env: OllamaHost, want: OllamaHost},
		{name: "bare host:port is normalised", env: "127.0.0.1:9999", want: "http://127.0.0.1:9999"},
		{name: "localhost counts as loopback", env: "http://localhost:11434", want: "http://localhost:11434"},
		{name: "trailing slash trimmed", env: "http://127.0.0.1:11434/", want: "http://127.0.0.1:11434"},
		{
			name:    "remote host is allowed but reported",
			env:     "http://ollama.example.com:11434",
			want:    "http://ollama.example.com:11434",
			warning: "not loopback",
		},
		{
			name:    "non-http scheme is rejected",
			env:     "ftp://127.0.0.1:11434",
			want:    OllamaHost,
			warning: "does not use http",
		},
		{
			name:    "unparseable value is rejected",
			env:     "http://[::1",
			want:    OllamaHost,
			warning: "not a valid URL",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			if tc.env == "" {
				t.Setenv("OLLAMA_HOST", "")
			} else {
				t.Setenv("OLLAMA_HOST", tc.env)
			}

			c := Load(Flags{})
			if c.OllamaHost != tc.want {
				t.Errorf("OllamaHost = %q, want %q", c.OllamaHost, tc.want)
			}
			if tc.warning != "" && !hasWarning(c, tc.warning) {
				t.Errorf("expected a warning containing %q, got %v", tc.warning, c.Warnings)
			}
			if tc.warning == "" && len(c.Warnings) != 0 {
				t.Errorf("expected no warnings, got %v", c.Warnings)
			}
		})
	}
}

func TestEnvIntIsClampedAtBothEnds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  int
		warn  bool
	}{
		{name: "in range", value: "4096", want: 4096},
		{name: "below the floor", value: "1", want: DefaultContextLen, warn: true},
		{name: "above the ceiling", value: "999999999", want: DefaultContextLen, warn: true},
		{name: "not a number", value: "lots", want: DefaultContextLen, warn: true},
		{name: "negative", value: "-5", want: DefaultContextLen, warn: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("DRAFT_NUM_CTX", tc.value)

			c := Load(Flags{})
			if c.ContextLength != tc.want {
				t.Errorf("ContextLength = %d, want %d", c.ContextLength, tc.want)
			}
			if got := hasWarning(c, "DRAFT_NUM_CTX"); got != tc.warn {
				t.Errorf("warning present = %v, want %v (warnings: %v)", got, tc.warn, c.Warnings)
			}
		})
	}
}

func TestResolveCallTimeout(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  string
		want time.Duration
		warn bool
	}{
		{name: "unset uses the default", env: "", want: DefaultCallTimeout},
		{name: "explicit seconds", env: "90", want: 90 * time.Second},
		{name: "zero disables the bound", env: "0", want: 0},
		{name: "negative is rejected", env: "-1", want: DefaultCallTimeout, warn: true},
		{name: "absurd is rejected", env: "99999999", want: DefaultCallTimeout, warn: true},
		{name: "not a number is rejected", env: "soon", want: DefaultCallTimeout, warn: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("DRAFT_CALL_TIMEOUT", tc.env)

			c := Load(Flags{})
			if c.CallTimeout != tc.want {
				t.Errorf("CallTimeout = %s, want %s", c.CallTimeout, tc.want)
			}
			if got := hasWarning(c, "DRAFT_CALL_TIMEOUT"); got != tc.warn {
				t.Errorf("warning present = %v, want %v (warnings: %v)", got, tc.warn, c.Warnings)
			}
		})
	}
}

func TestIsLoopback(t *testing.T) {
	for _, tc := range []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"127.1.2.3", true},
		{"::1", true},
		{"0.0.0.0", false},
		{"192.168.1.10", false},
		{"ollama.example.com", false},
		{"", false},
	} {
		if got := isLoopback(tc.host); got != tc.want {
			t.Errorf("isLoopback(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func hasWarning(c Config, substr string) bool {
	for _, w := range c.Warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}
