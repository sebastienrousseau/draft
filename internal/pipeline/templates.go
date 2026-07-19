package pipeline

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/sebastienrousseau/draft/internal/config"
	"github.com/sebastienrousseau/draft/internal/pdf"
)

const (
	maxTemplateChars = 3000
	maxTemplateFiles = 2
)

var headingLine = regexp.MustCompile(`(?m)^#{1,3} .+$`)

// loadTemplates builds an optional style-calibration block from the user's local
// template directory, if one exists. When absent (as on a fresh checkout), it
// returns "" and the prompt falls back to its built-in style example, so the
// tool works with zero local setup.
func loadTemplates(cfg config.Config) string {
	dirs := []string{
		filepath.Join(cfg.HomeDir, "Drop", "Templates"),
		filepath.Join(cfg.DraftsDir, "Templates"),
	}
	var dir string
	for _, d := range dirs {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			dir = d
			break
		}
	}
	if dir == "" {
		return ""
	}

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	sort.Slice(files, func(i, j int) bool { return modTime(files[i]) > modTime(files[j]) })
	if len(files) > maxTemplateFiles {
		files = files[:maxTemplateFiles]
	}

	var parts []string
	used := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		text := pdf.NormaliseSpace(string(b))
		if text == "" {
			continue
		}
		headings := strings.Join(firstN(headingLine.FindAllString(text, -1), 40), "\n")
		limit := 1500
		if rem := maxTemplateChars - used; rem < limit {
			limit = rem
		}
		if limit <= 0 {
			break
		}
		excerpt := text
		if len(excerpt) > limit {
			excerpt = excerpt[:limit]
		}
		parts = append(parts, "## Template example: "+filepath.Base(f)+"\n\n### Heading outline\n"+headings+"\n\n### Style sample\n"+excerpt)
		used += len(excerpt)
		if used >= maxTemplateChars {
			break
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func modTime(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.ModTime().UnixNano()
}

func firstN(in []string, n int) []string {
	if len(in) > n {
		return in[:n]
	}
	return in
}
