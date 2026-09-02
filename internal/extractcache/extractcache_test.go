// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package extractcache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	c := Open(t.TempDir())
	key := Key("section body", "v1", "claude", "sonnet")
	if _, ok := c.Get(key); ok {
		t.Fatal("empty cache returned a hit")
	}
	if err := c.Put(key, "CLAIM: x"); err != nil {
		t.Fatal(err)
	}
	got, ok := c.Get(key)
	if !ok || got != "CLAIM: x" {
		t.Errorf("Get() = %q, %v; want %q, true", got, ok, "CLAIM: x")
	}
}

// Every input that can change the output must change the address, or a prompt
// edit or model switch silently serves output produced by something else.
func TestKeyDependsOnEveryInput(t *testing.T) {
	base := Key("body", "v1", "claude", "sonnet")
	for _, tc := range []struct {
		name string
		key  string
	}{
		{"section", Key("other", "v1", "claude", "sonnet")},
		{"prompt version", Key("body", "v2", "claude", "sonnet")},
		{"engine", Key("body", "v1", "codex", "sonnet")},
		{"model", Key("body", "v1", "claude", "opus")},
	} {
		if tc.key == base {
			t.Errorf("changing the %s did not change the key", tc.name)
		}
	}
	if Key("body", "v1", "claude", "sonnet") != base {
		t.Error("Key is not deterministic")
	}
}

// Without a separator, ("ab","c") and ("a","bc") would address the same entry.
func TestKeyFieldsCannotRunTogether(t *testing.T) {
	if Key("ab", "c", "e", "m") == Key("a", "bc", "e", "m") {
		t.Error("adjacent fields collide")
	}
}

func TestExpiredEntryMissesAndIsRemoved(t *testing.T) {
	dir := t.TempDir()
	c := Open(dir)
	key := Key("body", "v1", "e", "m")
	if err := c.Put(key, "stale"); err != nil {
		t.Fatal(err)
	}
	// Rewrite the entry with an old timestamp.
	path := c.path(key)
	data, _ := json.Marshal(entry{Created: time.Now().Add(-MaxAge - time.Hour), Text: "stale"})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(key); ok {
		t.Error("expired entry was served")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expired entry was not removed")
	}
}

// A truncated write from an interrupted run must not become a permanent miss.
func TestCorruptEntryIsRemoved(t *testing.T) {
	dir := t.TempDir()
	c := Open(dir)
	key := Key("body", "v1", "e", "m")
	path := c.path(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(key); ok {
		t.Error("corrupt entry was served")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("corrupt entry was not removed")
	}
}

// Losing reuse is a slow run; a failed run is worse. Every disabled form must
// simply miss.
func TestDisabledCacheAlwaysMisses(t *testing.T) {
	for _, c := range []*Cache{nil, Open(""), {}} {
		if _, ok := c.Get(Key("b", "v", "e", "m")); ok {
			t.Error("disabled cache returned a hit")
		}
		if err := c.Put(Key("b", "v", "e", "m"), "x"); err != nil {
			t.Errorf("disabled cache Put returned %v", err)
		}
	}
	// A path that cannot be created is disabled rather than fatal.
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if c := Open(filepath.Join(file, "sub")); c != nil {
		t.Error("Open should return nil when the directory cannot be created")
	}
}

func TestClearRemovesShardsAndNothingElse(t *testing.T) {
	dir := t.TempDir()
	c := Open(dir)
	if err := c.Put(Key("body", "v1", "e", "m"), "x"); err != nil {
		t.Fatal(err)
	}
	// A mistyped DRAFT_CACHE_DIR must not delete someone's documents.
	keep := filepath.Join(dir, "important")
	if err := os.MkdirAll(keep, 0o700); err != nil {
		t.Fatal(err)
	}
	loose := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(loose, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Clear(dir); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(Key("body", "v1", "e", "m")); ok {
		t.Error("Clear left an entry behind")
	}
	for _, p := range []string{keep, loose} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("Clear removed unrelated path %s", p)
		}
	}
}

func TestClearToleratesAMissingDirectory(t *testing.T) {
	if err := Clear(filepath.Join(t.TempDir(), "never-created")); err != nil {
		t.Errorf("Clear() = %v, want nil", err)
	}
	if err := Clear(""); err != nil {
		t.Errorf("Clear(\"\") = %v, want nil", err)
	}
}

func TestClearReportsAnUnreadableDirectory(t *testing.T) {
	orig := readDir
	readDir = func(string) ([]os.DirEntry, error) { return nil, errors.New("permission denied") }
	defer func() { readDir = orig }()

	if err := Clear(t.TempDir()); err == nil {
		t.Error("Clear() should report a directory it cannot read")
	}
}

func TestPutReportsAnUnwritableShard(t *testing.T) {
	dir := t.TempDir()
	c := &Cache{dir: dir}
	key := Key("body", "v1", "e", "m")
	// Occupy the shard path with a file so MkdirAll fails.
	if err := os.WriteFile(filepath.Join(dir, key[:2]), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := c.Put(key, "x"); err == nil {
		t.Error("Put should report a shard that cannot be created")
	}
}

func TestIsHexRejectsNonShardNames(t *testing.T) {
	for _, s := range []string{"zz", "AB", "g0", "-1"} {
		if isHex(s) {
			t.Errorf("isHex(%q) = true", s)
		}
	}
	if !isHex("a0") {
		t.Error("isHex(\"a0\") = false")
	}
}
