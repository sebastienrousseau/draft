// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package extractcache stores the raw output of a claim-extraction call,
// addressed by the content that produced it.
//
// Extraction is 80-95% of a run's wall clock. The only reuse before this was
// --resume, whose ledger is named for today's date and the first source's
// filename — so redrafting the same paper tomorrow, or after renaming it,
// re-paid the whole cost. Addressing by content instead makes a redraft, a
// rename, and a --merge that overlaps an earlier run all free.
//
// A cache entry is never trusted on its own account. The pipeline still runs
// claims.Parse over the cached text against the freshly read section, so a
// stale or tampered entry can only ever produce fewer verified claims, never
// an ungrounded one. That is the same property that makes --resume safe.
package extractcache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/sebastienrousseau/draft/internal/atomicfile"
)

// readDir is os.ReadDir, as a variable so the failure path is testable on
// every platform. Reading a plain file as a directory errors on Unix and
// succeeds on Windows, so a test cannot portably provoke it for real.
var readDir = os.ReadDir

// MaxAge bounds how long an entry stays usable. Extraction output is
// deterministic given the same section, prompt, engine and model, so the limit
// is about reclaiming disk rather than correctness.
const MaxAge = 30 * 24 * time.Hour

// Cache is a content-addressed store under a single directory. The zero value
// and a nil *Cache are both usable and always miss, so a caller that could not
// open one does not need a branch at every call site.
type Cache struct {
	dir string
}

// entry is the on-disk record. The timestamp makes expiry possible and the
// text is stored as-is so the file is readable when debugging a bad ledger.
type entry struct {
	Created time.Time `json:"created"`
	Text    string    `json:"text"`
}

// Open returns a cache rooted at dir. An empty dir, or one that cannot be
// created, yields a cache that always misses: losing reuse is a slow run, and
// a slow run is a better outcome than a failed one.
func Open(dir string) *Cache {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil
	}
	return &Cache{dir: dir}
}

// Key derives the address of one extraction. Every input that can change the
// output is folded in: a prompt edit, an engine switch or a model change all
// produce a different key rather than silently serving output that different
// instructions produced.
func Key(section, promptVersion, engineName, model string) string {
	h := sha256.New()
	for _, part := range []string{promptVersion, engineName, model, section} {
		_, _ = h.Write([]byte(part))
		// A separator, so that ("ab","c") and ("a","bc") cannot collide.
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Get returns the cached extraction for key. A missing, unreadable, malformed
// or expired entry is a miss; an expired one is also removed.
func (c *Cache) Get(key string) (string, bool) {
	if c == nil || c.dir == "" {
		return "", false
	}
	path := c.path(key)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var e entry
	if err := json.Unmarshal(data, &e); err != nil {
		// A truncated write from an interrupted run. Drop it rather than
		// leaving a file that can never be a hit.
		_ = os.Remove(path)
		return "", false
	}
	if time.Since(e.Created) > MaxAge {
		_ = os.Remove(path)
		return "", false
	}
	return e.Text, true
}

// Put stores text under key. A write failure is reported but is never fatal to
// a run: the work it would have saved has already been done.
func (c *Cache) Put(key, text string) error {
	if c == nil || c.dir == "" {
		return nil
	}
	path := c.path(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(entry{Created: time.Now(), Text: text})
	if err != nil {
		return err
	}
	// Stage and rename, so a parallel extraction worker reading this key
	// never sees a half-written entry — Get treats malformed JSON as corrupt
	// and deletes it, which would otherwise let a reader destroy a writer's
	// file mid-write.
	return atomicfile.Write(path, data, 0o600)
}

// Clear removes every entry. It refuses a directory that does not look like a
// cache, so a mistyped DRAFT_CACHE_DIR cannot delete something else.
func Clear(dir string) error {
	if dir == "" {
		return nil
	}
	entries, err := readDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		// Shards are two hex characters; anything else was not put here by
		// this package and is left alone.
		if !e.IsDir() || len(e.Name()) != 2 || !isHex(e.Name()) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// path shards on the first two characters so one directory does not
// accumulate every entry a machine has ever produced.
func (c *Cache) path(key string) string {
	return filepath.Join(c.dir, key[:2], key+".json")
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
