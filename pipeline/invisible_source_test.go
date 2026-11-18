// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sebastienrousseau/draft/engine"
)

// A source can hide text from its reader with tag characters and bidi
// controls. The extraction prompt must carry the visible text only, and a
// quote of that visible text must still verify against the section it came
// from, so the model and the verifier agree on what the source says.
func TestInvisibleSourceTextNeverReachesTheModel(t *testing.T) {
	src := filepath.Join(t.TempDir(), "source.txt")
	hidden := "The system reached a score\U000E0049\U000E0047\U000E004E\U000E004F\U000E0052\U000E0045 of 0.82 on the \u202Etest\u202C set. It used 5x fewer samples than before."
	if err := os.WriteFile(src, []byte(hidden), 0o644); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var prompts []string
	eng := funcEngine{name: "fake", gen: func(_ context.Context, req engine.Request) (engine.Result, error) {
		if req.Kind == engine.KindExtract {
			mu.Lock()
			prompts = append(prompts, req.Prompt)
			mu.Unlock()
			return engine.Result{Text: extractionResponse}, nil
		}
		return engine.Result{Text: validArticle(".")}, nil
	}}
	done, errText, _ := drain(t, testConfig(t), []engine.Engine{eng}, Job{Sources: []string{src}})
	if errText != "" {
		t.Fatalf("the visible quote should verify and the draft save: %s", errText)
	}
	if done.OutputPath == "" {
		t.Fatal("nothing was saved")
	}
	if len(prompts) == 0 {
		t.Fatal("no extraction prompt was seen")
	}
	for _, p := range prompts {
		if strings.ContainsAny(p, "\U000E0049\u202E\u202C") {
			t.Errorf("an extraction prompt carries invisible source text: %+q", p)
		}
	}
}
