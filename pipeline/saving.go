// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/engine"
	"github.com/sebastienrousseau/draft/frontmatter"
	"github.com/sebastienrousseau/draft/internal/pdf"
	"github.com/sebastienrousseau/draft/prompt"
	"github.com/sebastienrousseau/draft/provenance"
	"github.com/sebastienrousseau/draft/validate"
)

// writerModel is the model the writing backend used, for the run manifest.
func (r *Runner) writerModel() string {
	if r.wroteWith != nil {
		return engine.ResolveModel(r.cfg, r.wroteWith)
	}
	return engine.ResolveModel(r.cfg, r.chainFor(engine.KindWrite).active())
}

// writerName is the engine that produced the article, which is the one worth
// reporting when extraction and writing may be different backends.
func (r *Runner) writerName() string {
	if r.wroteWith != nil {
		return r.wroteWith.Name()
	}
	if e := r.chainFor(engine.KindWrite).active(); e != nil {
		return e.Name()
	}
	return r.engineName
}

// save writes the article as a day-folder set — source/<stem>-body.md,
// yaml/<stem>-frontmatter.yaml and final/<stem>-final.md under outputDir —
// and returns the final document's path.
func (r *Runner) save(outputDir, markdown string, records []claims.Record) (string, int, error) {
	_, body := frontmatter.Split(markdown)
	body = strings.TrimSpace(body)

	title := extractTitle(body)
	now := time.Now()
	dateStr := now.Format("2006-01-02")
	base := dateStr + "-" + slugify(title)

	srcDir := filepath.Join(outputDir, "source")
	yamlDir := filepath.Join(outputDir, "yaml")
	finalDir := filepath.Join(outputDir, "final")
	for _, d := range []string{srcDir, yamlDir, finalDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", 0, err
		}
	}

	// Uniquify the trio as a set: a leftover in any one folder bumps all three
	// names together, so the files can never desync.
	//
	// The body file is claimed with O_EXCL rather than a bare existence check,
	// so two runs racing on the same title cannot both pick the same stem
	// between the check and the write. The loop is bounded: an unbounded
	// search would spin forever against an undeletable path.
	var stem, bodyPath, fmPath, finalPath string
	var bodyFile *os.File
	for i := 1; ; i++ {
		if i > maxStemAttempts {
			return "", 0, fmt.Errorf("could not find a free filename for %q after %d attempts", base, maxStemAttempts)
		}
		stem = base
		if i > 1 {
			stem = fmt.Sprintf("%s-%d", base, i)
		}
		bodyPath = filepath.Join(srcDir, stem+"-body.md")
		fmPath = filepath.Join(yamlDir, stem+"-frontmatter.yaml")
		finalPath = filepath.Join(finalDir, stem+"-final.md")
		if fileExists(fmPath) || fileExists(finalPath) {
			continue
		}
		f, err := os.OpenFile(bodyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", 0, err
		}
		bodyFile = f
		break
	}

	if _, err := bodyFile.WriteString(body + "\n"); err != nil {
		_ = bodyFile.Close()
		return "", 0, err
	}
	if err := bodyFile.Close(); err != nil {
		return "", 0, err
	}
	site := frontmatter.SiteFromEnv()
	fmYAML := frontmatter.GenerateWithOptions(body, frontmatter.Options{
		Date:    now,
		Slug:    strings.TrimPrefix(stem, dateStr+"-"),
		Site:    &site,
		Engine:  r.writerName(),
		Model:   r.writerModel(),
		Version: r.cfg.Version,
	})
	if err := os.WriteFile(fmPath, []byte(fmYAML+"\n"), 0o644); err != nil {
		return "", 0, err
	}
	finalMD := frontmatter.Combine(fmYAML, body)
	if err := os.WriteFile(finalPath, []byte(finalMD+"\n"), 0o644); err != nil {
		return "", 0, err
	}

	r.log("saved body: " + shortPath(r.cfg, bodyPath))
	r.log("saved frontmatter: " + shortPath(r.cfg, fmPath))
	r.saveProvenance(outputDir, stem, title, body+"\n", records, now)

	return finalPath, validate.WordCount(body), nil
}

// saveProvenance writes the attribution and the C2PA manifest definition for
// a saved body. Neither can fail the job: the article is already on disk and
// grounded, and a missing provenance file is a warning the reader can see,
// not a reason to throw the article away.
func (r *Runner) saveProvenance(outputDir, stem, title, saved string, records []claims.Record, now time.Time) {
	dir := filepath.Join(outputDir, "provenance")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.warn("could not create the provenance directory: " + err.Error())
		return
	}
	att := provenance.Attribute(saved, records)
	reader := r.cfg.Reader
	if reader == "" {
		reader = pdf.ReaderPDFToText
	}
	sources := make([]provenance.Source, 0, len(r.sourceDigests))
	for _, s := range r.sourceDigests {
		sources = append(sources, provenance.Source{Path: s.Path, SHA256: s.SHA256})
	}
	man := provenance.NewManifest(provenance.ManifestInput{
		Title: title, Body: saved, Version: r.cfg.Version,
		Engine: r.writerName(), Model: r.writerModel(), Reader: reader,
		PromptVersion: prompt.ClaimVersion(), LedgerSHA256: r.ledgerDigest,
		Sources: sources, When: now, Attribution: &att,
	})
	write := func(name string, v any) string {
		path := filepath.Join(dir, stem+name)
		b, err := json.MarshalIndent(v, "", "  ")
		if err == nil {
			err = os.WriteFile(path, append(b, '\n'), 0o644)
		}
		if err != nil {
			r.warn(fmt.Sprintf("could not save %s: %v", filepath.Base(path), err))
			return ""
		}
		return path
	}
	r.attributionPath = write("-attribution.json", att)
	r.manifestPath = write("-c2pa.json", man)
	r.sentences, r.attributed = len(att.Sentences), att.Attributed
	if r.attributionPath != "" {
		r.log(fmt.Sprintf("saved attribution: %s (%d of %d sentences rest on a claim)", shortPath(r.cfg, r.attributionPath), att.Attributed, len(att.Sentences)))
	}
	if r.manifestPath != "" {
		r.log("saved manifest: " + shortPath(r.cfg, r.manifestPath))
	}
}

// saveFailure preserves the raw output and, if it still looks like an article,
// a needs-review copy, then returns an error describing where they went.
// It reports only the files it actually managed to write: telling the user
// where to find a rescued draft that was never saved sends them looking for a
// path that does not exist.
func (r *Runner) saveFailure(outputDir, markdown string, verr error) error {
	var note strings.Builder
	rawPath := filepath.Join(outputDir, time.Now().Format("2006-01-02")+"-failed-output.txt")
	if err := os.WriteFile(rawPath, []byte(markdown+"\n"), 0o644); err != nil {
		note.WriteString("\nRaw output could not be saved: " + err.Error())
	} else {
		note.WriteString("\nRaw output saved: " + rawPath)
	}
	if validate.LooksLikeArticle(markdown) {
		reviewPath := uniquePath(filepath.Join(outputDir, time.Now().Format("2006-01-02")+"-"+slugify(extractTitle(markdown))+"-needs-review.md"))
		if err := os.WriteFile(reviewPath, []byte(markdown+"\n"), 0o644); err != nil {
			note.WriteString("\nNeeds-review Markdown could not be saved: " + err.Error())
		} else {
			note.WriteString("\nNeeds-review Markdown saved: " + reviewPath)
		}
	}
	return fmt.Errorf("%w%s", verr, note.String())
}

func (r *Runner) datedDir() string {
	return filepath.Join(r.cfg.DraftsDir, time.Now().Format("2006-01-02"))
}
