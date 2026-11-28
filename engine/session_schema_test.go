// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/config"
)

var testSchema = map[string]any{"type": "object", "properties": map[string]any{"claims": map[string]any{"type": "array"}}}

func schemaJSON(t *testing.T) string {
	t.Helper()
	b, err := json.Marshal(testSchema)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Claude takes the schema inline with --json-schema; Codex takes a file path
// with --output-schema. Both were verified live against the real CLIs, which
// accept these flags and answer with the object the schema describes.
func TestSessionPassesTheSchemaTheWayEachProviderTakesIt(t *testing.T) {
	want := schemaJSON(t)
	withExec("echo-schema", func() {
		s, _ := NewSession("codex", config.Config{})
		res, err := s.Generate(context.Background(), Request{Prompt: "p", Schema: testSchema})
		if err != nil {
			t.Fatal(err)
		}
		path, body, ok := strings.Cut(strings.TrimPrefix(res.Text, "FILE:"), "\x1f")
		if !strings.HasPrefix(res.Text, "FILE:") || !ok || body != want {
			t.Fatalf("codex got %q, want the schema in a file", res.Text)
		}
		if !strings.Contains(path, "draft-session-") {
			t.Errorf("schema file %q is not in the call's sandbox directory", path)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("schema file %q outlived the call", path)
		}
	})
	withExec("echo-schema", func() {
		s, _ := NewSession("claude", config.Config{})
		res, err := s.Generate(context.Background(), Request{Prompt: "p", Schema: testSchema})
		if err != nil {
			t.Fatal(err)
		}
		if res.Text != "INLINE:"+want {
			t.Errorf("claude got %q, want the schema inline", res.Text)
		}
	})
}

// No schema means no flag, and a provider that cannot take a schema never
// gets one, even when a request carries it.
func TestSessionSendsNoSchemaFlagWithoutCause(t *testing.T) {
	for _, tc := range []struct {
		provider string
		schema   map[string]any
	}{
		{"codex", nil},
		{"claude", nil},
		{"crush", testSchema},
	} {
		withExec("echo-args", func() {
			s, ok := NewSession(tc.provider, config.Config{})
			if !ok {
				t.Fatalf("%s provider should exist", tc.provider)
			}
			res, _ := s.Generate(context.Background(), Request{Prompt: "p", Schema: tc.schema})
			if strings.Contains(res.Text, "schema") {
				t.Errorf("%s with schema %v was given a schema flag: %q", tc.provider, tc.schema != nil, res.Text)
			}
		})
	}
}

func TestSessionSchemaCapability(t *testing.T) {
	for name, want := range map[string]bool{"claude": true, "codex": true, "crush": false, "copilot": false} {
		s, ok := NewSession(name, config.Config{})
		if !ok {
			t.Fatalf("%s provider should exist", name)
		}
		if got := SupportsSchema(s); got != want {
			t.Errorf("SupportsSchema(%s) = %v, want %v", name, got, want)
		}
	}
}
