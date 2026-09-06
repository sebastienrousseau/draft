// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package provenance turns a finished article and its verified claim ledger
// into artefacts a reader can check: a per-sentence attribution that names
// which claims each sentence rests on, and a C2PA manifest definition that
// binds the article, its sources, its ledger and the backend that wrote it.
//
// The ledger already proves the article as a whole is grounded. What it
// cannot show is which claim backs sentence 14, or that the file a reader is
// holding is the one the ledger was verified against. Attribution answers the
// first; the manifest answers the second, once a publisher signs it.
//
// Attribution is computed after the fact from the text alone, deterministic
// and model-free: a sentence is tied to a claim when they share a number and
// a content word, or enough content words that the overlap is not chance.
// It is evidence of provenance, not proof of it, and it says so in its own
// field names.
package provenance

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sebastienrousseau/draft/claims"
	"github.com/sebastienrousseau/draft/rules"
)

// Schema versions the attribution and grounding-assertion shapes. Bump it
// when a field changes meaning or disappears, never for a pure addition.
const Schema = 1

// Claim is a verified claim with the identifier attribution refers to it by.
type Claim struct {
	ID       string `json:"id"`
	Claim    string `json:"claim"`
	Quote    string `json:"quote"`
	Type     string `json:"type,omitempty"`
	Strength string `json:"strength,omitempty"`
}

// Sentence is one sentence of the article body and the claims it rests on.
type Sentence struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
	// Start and End are byte offsets into the body the attribution was
	// computed over, so a tool can highlight the sentence in place.
	Start int `json:"start"`
	End   int `json:"end"`
	// Claims lists the identifiers of the claims this sentence draws on,
	// strongest evidence first. Empty means the sentence is framing,
	// transition or structure rather than a fact.
	Claims []string `json:"claims,omitempty"`
	// Evidence is how the attribution was made: "number" when the sentence
	// and claim share a figure and a content word, "lexical" when they share
	// enough content words. Empty when unattributed.
	Evidence string `json:"evidence,omitempty"`
	// UngroundedNumbers lists figures in the sentence that appear in no
	// verified claim at all. It is the same signal --strict-numbers fails a
	// draft on, reported per sentence rather than per article.
	UngroundedNumbers []string `json:"ungrounded_numbers,omitempty"`
}

// Attribution is the per-sentence map from article to ledger.
type Attribution struct {
	Schema        int        `json:"schema"`
	ArticleSHA256 string     `json:"article_sha256"`
	Sentences     []Sentence `json:"sentences"`
	Claims        []Claim    `json:"claims"`
	// Attributed and Unattributed count sentences with and without a claim.
	// Framing and transitions are expected to be unattributed; a low
	// attributed count on a long article is worth a look, not a failure.
	Attributed   int `json:"attributed"`
	Unattributed int `json:"unattributed"`
	// SentencesWithUngroundedNumbers counts sentences carrying a figure no
	// claim contains.
	SentencesWithUngroundedNumbers int `json:"sentences_with_ungrounded_numbers"`
}

// ClaimID is the stable identifier of a verified claim: "c" followed by the
// first ten hex digits of the SHA-256 of its normalised quote. Two ledgers
// that verified the same quote name it the same way, and a reader with the
// ledger can recompute it.
func ClaimID(rec claims.Record) string {
	sum := sha256.Sum256([]byte(normalise(rec.SourceQuote)))
	return "c" + hex.EncodeToString(sum[:])[:10]
}

// Claims assigns identifiers to a ledger, in ledger order.
func Claims(records []claims.Record) []Claim {
	out := make([]Claim, 0, len(records))
	for _, rec := range records {
		out = append(out, Claim{ID: ClaimID(rec), Claim: rec.Claim, Quote: rec.SourceQuote, Type: rec.Type, Strength: rec.Strength})
	}
	return out
}

// Attribute maps every prose sentence of body to the claims it rests on.
func Attribute(body string, records []claims.Record) Attribution {
	sum := sha256.Sum256([]byte(body))
	att := Attribution{Schema: Schema, ArticleSHA256: hex.EncodeToString(sum[:]), Claims: Claims(records)}

	type indexed struct {
		id      string
		tokens  map[string]bool
		numbers map[string]bool
	}
	index := make([]indexed, 0, len(records))
	allNumbers := map[string]bool{}
	for _, rec := range records {
		nums := claims.Numbers(rec.SourceQuote)
		for n := range claims.Numbers(rec.Claim) {
			nums[n] = true
		}
		for n := range nums {
			allNumbers[n] = true
		}
		index = append(index, indexed{id: ClaimID(rec), tokens: tokens(rec.Claim + " " + rec.SourceQuote), numbers: nums})
	}

	for _, span := range sentences(body) {
		s := Sentence{Index: len(att.Sentences), Text: span.text, Start: span.start, End: span.end}
		sTokens := tokens(span.text)
		sNumbers := claims.Numbers(span.text)
		for n := range sNumbers {
			if !allNumbers[n] {
				s.UngroundedNumbers = append(s.UngroundedNumbers, n)
			}
		}
		sort.Strings(s.UngroundedNumbers)

		type hit struct {
			id       string
			score    int
			evidence string
		}
		var hits []hit
		for _, c := range index {
			shared := 0
			for t := range sTokens {
				if c.tokens[t] {
					shared++
				}
			}
			numbers := 0
			for n := range sNumbers {
				if c.numbers[n] {
					numbers++
				}
			}
			switch {
			case numbers > 0 && shared > 0:
				hits = append(hits, hit{c.id, 100*numbers + shared, "number"})
			case shared >= minSharedTokens && shared*100 >= minSharedPercent*len(sTokens):
				hits = append(hits, hit{c.id, shared, "lexical"})
			}
		}
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
		if len(hits) > maxClaimsPerSentence {
			hits = hits[:maxClaimsPerSentence]
		}
		for _, h := range hits {
			s.Claims = append(s.Claims, h.id)
		}
		if len(hits) > 0 {
			s.Evidence = hits[0].evidence
			att.Attributed++
		} else {
			att.Unattributed++
		}
		if len(s.UngroundedNumbers) > 0 {
			att.SentencesWithUngroundedNumbers++
		}
		att.Sentences = append(att.Sentences, s)
	}
	return att
}

const (
	// minSharedTokens and minSharedPercent set the lexical threshold: at
	// least this many content words in common, and at least this share of
	// the sentence's own content words. Below it two sentences on the same
	// topic match by coincidence; above it they are saying the same thing.
	minSharedTokens  = 3
	minSharedPercent = 35
	// maxClaimsPerSentence keeps the map readable. A sentence that really
	// draws on four claims is rare; one that "matches" six is noise.
	maxClaimsPerSentence = 3
)

var (
	wordPat     = regexp.MustCompile(`[\p{L}][\p{L}\p{N}'-]{2,}`)
	smartQuotes = strings.NewReplacer("“", `"`, "”", `"`, "‘", "'", "’", "'")
	// leadMarker strips list bullets, blockquote bars and their combinations
	// from the start of a line, keeping the byte count so offsets stay true.
	// A bullet counts only when followed by whitespace, so a line that opens
	// in bold ("**A single number...**") is prose, not a list.
	leadMarker = regexp.MustCompile(`^(?:\s*(?:>|[-*+]\s|\d+[.)]\s))*\s*`)
)

// normalise matches claims.normalise so the identifier is computed over the
// same text the quote was verified with.
func normalise(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(smartQuotes.Replace(s)), " "))
}

// tokens returns the content words of s: lower-cased, stopwords removed,
// a trailing plural s dropped so "agents" and "agent" agree.
func tokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range wordPat.FindAllString(strings.ToLower(s), -1) {
		w = strings.Trim(w, "'-")
		if len(w) < 3 || rules.WriterStopwords[w] {
			continue
		}
		out[strings.TrimSuffix(w, "s")] = true
	}
	return out
}

// span is a sentence with its position in the body.
type span struct {
	text       string
	start, end int
}

// sentences splits the prose of a Markdown body into sentences with byte
// offsets. Headings, HTML, tables, horizontal rules and fenced code are
// structure, not statements, and are skipped; list and quote markers are
// stripped from the front of a line. A full stop ends a sentence only when
// followed by space or the end of the line, so "0.82" stays whole.
func sentences(body string) []span {
	var out []span
	offset := 0
	inFence := false
	for _, line := range strings.SplitAfter(body, "\n") {
		lineStart := offset
		offset += len(line)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || trimmed == "" {
			continue
		}
		// A list item or a quote is prose behind a marker; a heading, tag,
		// table row or rule is structure. Strip the markers first, then
		// judge what is left.
		rest := leadMarker.ReplaceAllString(line, "")
		restTrim := strings.TrimSpace(rest)
		if restTrim == "" || strings.Trim(restTrim, "-*_ ") == "" {
			continue
		}
		switch restTrim[0] {
		case '#', '<', '|':
			continue
		}
		if strings.HasPrefix(restTrim, "**Executive Summary**") {
			continue
		}
		lineStart += len(line) - len(rest)
		line = rest
		start := 0
		for i := 0; i < len(line); i++ {
			c := line[i]
			if c != '.' && c != '!' && c != '?' {
				continue
			}
			if i+1 < len(line) && line[i+1] != ' ' && line[i+1] != '\n' && line[i+1] != '\t' && line[i+1] != '\r' {
				continue
			}
			if seg := strings.TrimSpace(line[start : i+1]); seg != "" {
				lead := strings.Index(line[start:i+1], seg)
				out = append(out, span{text: seg, start: lineStart + start + lead, end: lineStart + start + lead + len(seg)})
			}
			start = i + 1
		}
		if seg := strings.TrimSpace(line[start:]); seg != "" {
			lead := strings.Index(line[start:], seg)
			out = append(out, span{text: seg, start: lineStart + start + lead, end: lineStart + start + lead + len(seg)})
		}
	}
	return out
}

// Source is one input document and the digest of the bytes that were read.
type Source struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// ManifestInput is everything the manifest binds together.
type ManifestInput struct {
	Title         string
	Body          string
	Version       string
	Engine        string
	Model         string
	Reader        string
	PromptVersion string
	LedgerSHA256  string
	Sources       []Source
	When          time.Time
	Attribution   *Attribution
}

// Manifest is a C2PA manifest definition in the shape c2patool reads: the
// generator, a created action naming the software agent, and a grounding
// assertion under draft's own label. It is a definition, not a signed
// manifest: draft holds no signing key, and embedding a manifest is the
// publisher's step (c2patool -m <this file>). Everything a signer needs to
// bind the article to its evidence is here.
type Manifest struct {
	ClaimGeneratorInfo []Generator  `json:"claim_generator_info"`
	Title              string       `json:"title"`
	Format             string       `json:"format"`
	Assertions         []Assertion  `json:"assertions"`
	Ingredients        []Ingredient `json:"ingredients,omitempty"`
}

// Generator names the software that produced the manifest.
type Generator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Assertion is one labelled statement in the manifest.
type Assertion struct {
	Label string `json:"label"`
	Data  any    `json:"data"`
}

// Ingredient is one input to the article.
type Ingredient struct {
	Title        string `json:"title"`
	Relationship string `json:"relationship"`
	Format       string `json:"format,omitempty"`
}

// GroundingLabel is the assertion label draft's own evidence sits under.
const GroundingLabel = "com.draftlib.grounding"

// Grounding is the data of the com.draftlib.grounding assertion.
type Grounding struct {
	Schema                         int      `json:"schema"`
	ArticleSHA256                  string   `json:"article_sha256"`
	LedgerSHA256                   string   `json:"ledger_sha256,omitempty"`
	PromptVersion                  string   `json:"prompt_version,omitempty"`
	Engine                         string   `json:"engine,omitempty"`
	Model                          string   `json:"model,omitempty"`
	Reader                         string   `json:"reader,omitempty"`
	Sources                        []Source `json:"sources,omitempty"`
	Claims                         []string `json:"claims,omitempty"`
	Sentences                      int      `json:"sentences"`
	Attributed                     int      `json:"attributed"`
	Unattributed                   int      `json:"unattributed"`
	SentencesWithUngroundedNumbers int      `json:"sentences_with_ungrounded_numbers"`
}

// digitalSourceType is the IPTC term for content produced by a trained
// algorithm; it is what the C2PA actions assertion expects for model output.
const digitalSourceType = "http://cv.iptc.org/newscodes/digitalsourcetype/trainedAlgorithmicMedia"

// NewManifest builds the manifest definition for one article.
func NewManifest(in ManifestInput) Manifest {
	sum := sha256.Sum256([]byte(in.Body))
	when := in.When
	if when.IsZero() {
		when = time.Now()
	}
	g := Grounding{
		Schema:        Schema,
		ArticleSHA256: hex.EncodeToString(sum[:]),
		LedgerSHA256:  in.LedgerSHA256,
		PromptVersion: in.PromptVersion,
		Engine:        in.Engine,
		Model:         in.Model,
		Reader:        in.Reader,
		Sources:       in.Sources,
	}
	if in.Attribution != nil {
		for _, c := range in.Attribution.Claims {
			g.Claims = append(g.Claims, c.ID)
		}
		g.Sentences = len(in.Attribution.Sentences)
		g.Attributed = in.Attribution.Attributed
		g.Unattributed = in.Attribution.Unattributed
		g.SentencesWithUngroundedNumbers = in.Attribution.SentencesWithUngroundedNumbers
	}
	m := Manifest{
		ClaimGeneratorInfo: []Generator{{Name: "draft", Version: in.Version}},
		Title:              in.Title,
		Format:             "text/markdown",
		Assertions: []Assertion{
			{Label: "c2pa.actions", Data: map[string]any{"actions": []map[string]any{{
				"action":            "c2pa.created",
				"digitalSourceType": digitalSourceType,
				"softwareAgent":     map[string]string{"name": "draft", "version": in.Version},
				"when":              when.UTC().Format(time.RFC3339),
			}}}},
			{Label: GroundingLabel, Data: g},
		},
	}
	for _, s := range in.Sources {
		m.Ingredients = append(m.Ingredients, Ingredient{Title: baseName(s.Path), Relationship: "inputTo", Format: formatOf(s.Path)})
	}
	return m
}

func baseName(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}

func formatOf(path string) string {
	switch strings.ToLower(path[strings.LastIndex(path, ".")+1:]) {
	case "pdf":
		return "application/pdf"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "md":
		return "text/markdown"
	case "txt":
		return "text/plain"
	}
	return ""
}
