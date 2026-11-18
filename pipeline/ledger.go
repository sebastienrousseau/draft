// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package pipeline

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/internal/atomicfile"
)

// saveLedger writes the ledger the article is about to be written from and
// records its digest for provenance. A resumed run writes one too: the
// records it kept may be fewer than the file it read held, and the manifest's
// ledger_sha256 must name the ledger that was used, not the one it came from.
//
// The write goes through a rename, so a concurrent --resume reading the same
// folder sees a whole ledger or none, never a truncated one. The ledger
// quotes the user's documents verbatim and is scratch rather than a
// publication, so it is readable by the user alone.
//
// Report what actually happened. Logging "claims saved" unconditionally
// after a discarded write tells the user a fact-checking artefact exists
// when it may not.
func (r *Runner) saveLedger(path string, records []claims.Record, dropped int) {
	text := claims.RenderLedger(records, dropped) + "\n"
	r.ledgerDigest = sha256Hex(text)
	r.ledgerPath = path
	if err := atomicfile.Write(path, []byte(text), 0o600); err != nil {
		r.ledgerPath = ""
		r.warn(fmt.Sprintf("could not save the claim ledger: %v", err))
		return
	}
	r.log("claims saved to " + shortPath(r.cfg, path))
}

// ownLedgerPath names the verified-claim ledger this run writes.
//
// The name carries a token drawn once per run. Without it, two runs of the
// same paper on the same day wrote one file, and the first to succeed
// deleted the ledger the other was still writing its article from.
func (r *Runner) ownLedgerPath(outputDir string, job Job) string {
	if r.runToken == "" {
		var b [4]byte
		// crypto/rand.Read is documented never to return an error since Go 1.24.
		_, _ = rand.Read(b[:])
		r.runToken = hex.EncodeToString(b[:])
	}
	return filepath.Join(outputDir, ledgerName(job, r.runToken))
}

// ledgerName is a ledger's file name for today. The source stem keeps two
// papers drafted the same day apart, and the source count keeps a --merge
// job apart from a single-source job that starts from the same file. An
// empty token gives the name ledgers had before they carried one.
func ledgerName(job Job, token string) string {
	name := time.Now().Format("2006-01-02") + "-" + LedgerStem(job.Sources)
	if token != "" {
		name += "." + token
	}
	return name + LedgerSuffix
}

// LedgerSuffix ends every verified-claim ledger's file name.
const LedgerSuffix = "-verified-claim-ledger.md"

// LedgerStem is the part of a ledger's name, between the date and
// LedgerSuffix, that identifies the job's sources. It is exported so --verify
// can tell a set's own ledger from another paper's in the same day folder.
func LedgerStem(sources []string) string {
	stem := "sources"
	if len(sources) > 0 {
		base := filepath.Base(sources[0])
		stem = slugify(strings.TrimSuffix(base, filepath.Ext(base)))
	}
	if len(sources) > 1 {
		stem = fmt.Sprintf("%s-plus-%d", stem, len(sources)-1)
	}
	return stem
}

// IsLedgerFor reports whether name is a ledger the pipeline wrote, on any
// day, for a job over these sources: <date>-<stem>[.<run token>] followed by
// LedgerSuffix. A slug never contains a dot, so the token is unambiguous.
func IsLedgerFor(name string, sources []string) bool {
	rest, ok := strings.CutSuffix(name, LedgerSuffix)
	if !ok {
		return false
	}
	if i := strings.LastIndexByte(rest, '.'); i >= 0 {
		if !isRunToken(rest[i+1:]) {
			return false
		}
		rest = rest[:i]
	}
	date, ok := strings.CutSuffix(rest, "-"+LedgerStem(sources))
	if !ok {
		return false
	}
	_, err := time.Parse("2006-01-02", date)
	return err == nil
}

func isRunToken(s string) bool {
	if s == "" {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil && strings.ToLower(s) == s
}

// resumableLedger finds today's ledger for the job's sources, if any. With a
// ledger per run there may be several, from earlier failed attempts; the
// newest is the attempt the user is retrying. Whichever is picked is only a
// candidate: resume re-verifies it against the sources before using any of it.
func resumableLedger(outputDir string, job Job) (string, bool) {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return "", false
	}
	today := time.Now().Format("2006-01-02") + "-"
	var best string
	var newest time.Time
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || !strings.HasPrefix(name, today) || !IsLedgerFor(name, job.Sources) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(newest) {
			best, newest = filepath.Join(outputDir, name), info.ModTime()
		}
	}
	return best, best != ""
}

// cleanupArtifacts removes the scratch claim ledger after a successful draft so
// the dated folder holds only finished articles, along with the ledger a
// resumed run consumed. Each run removes only files it wrote or read, never
// another run's. The --keep-artifacts flag preserves them for fact-checking.
func (r *Runner) cleanupArtifacts() {
	if r.cfg.KeepArtifacts {
		return
	}
	removed := false
	for _, p := range []string{r.ledgerPath, r.resumedFrom} {
		if p != "" && os.Remove(p) == nil {
			removed = true
		}
	}
	if removed {
		r.log("cleaned up claim ledger (use --keep-artifacts to keep it)")
	}
}
