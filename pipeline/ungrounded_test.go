// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sebastienrousseau/draft/engine"
)

// With no verified claim there is nothing the writer may state, so there is
// nothing to write. The faithfulness checks only see numbers and metric terms,
// so an article written from an empty ledger passes them on invented prose.
func TestNoVerifiedClaimsWritesNothing(t *testing.T) {
	cases := map[string]string{
		"model found none":   "NONE",
		"none verified":      "CLAIM: The system scored 0.99\nSOURCE_QUOTE: \"a quote the source never had\"\nTYPE: metric\nSTRENGTH: demonstrated\n---",
		"malformed response": "I could not find any claims in this section.",
	}
	for name, response := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := testConfig(t)
			eng := &fakeEngine{name: "fake", extractResponse: response, writer: func(int) (string, bool) {
				return validArticle(""), false
			}}
			r := NewRunner(cfg, []engine.Engine{eng}, nil)
			err := r.run(context.Background(), Job{Sources: []string{writeSource(t)}})
			if !errors.Is(err, ErrNoVerifiedClaims) {
				t.Fatalf("err = %v, want ErrNoVerifiedClaims", err)
			}
			if eng.writeCalls != 0 {
				t.Errorf("the writer was called %d time(s) with no verified claims", eng.writeCalls)
			}
			if saved := savedFiles(t, cfg.DraftsDir); len(saved) != 0 {
				t.Errorf("files saved from an empty ledger: %v", saved)
			}
		})
	}
}

// savedFiles lists every article-set file under dir: the body, frontmatter
// and final document a successful run writes.
func savedFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		switch filepath.Base(filepath.Dir(path)) {
		case "source", "yaml", "final":
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
