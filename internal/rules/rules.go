// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package rules holds the shared editorial constants that both the prompt
// builder and the validator depend on: word limits, the banned vocabulary,
// metric vocabulary, and the accepted claim taxonomy. Keeping them in one
// place guarantees the writer is told exactly what the validator enforces.
package rules

// Word-count bounds for a finished body-only draft.
const (
	MinWords = 500
	MaxWords = 3000
)

// MinQuoteChars is the shortest verbatim source span a claim may cite.
const MinQuoteChars = 12

// BannedWords are single tokens the house style forbids. They are matched on
// word boundaries, case-insensitively.
var BannedWords = []string{
	"delve", "underscore", "testament", "foster", "maximize", "navigate",
	"tapestry", "catalyst", "elevate", "paradigm", "revolutionize",
	"paramount", "leverage", "harness", "unlock", "seamless", "robust",
	"realm", "landscape", "beacon", "game-changer", "cutting-edge",
	"utilize", "myriad", "vibrant", "bustling", "whimsical", "profound",
}

// BannedPhrases are multi-word clichés the house style forbids.
var BannedPhrases = []string{
	"in today's fast-paced world", "at its core", "it's important to note",
	"furthermore", "moreover", "in conclusion", "in summary", "let's face it",
	"but here's the kicker", "when it comes to", "in the realm of",
	"look no further", "rest assured", "needless to say", "not only",
	"but also", "whether you're", "that said", "in essence", "ultimately",
	"paradigm shift", "the dawn of", "in today's world",
}

// StyleReplacements maps every banned word and phrase to a neutral, in-style
// equivalent, so a draft that trips the banned-vocabulary check can be repaired
// in place rather than regenerated — regeneration is slow and, on a small local
// model, simply produces fresh clichés. Every replacement is itself outside the
// banned lists; matching is case-insensitive and the replacement takes the case
// of the first matched character. Phrases are applied before single words, so a
// phrase that contains a banned word (for example "in the realm of") is rewritten
// whole. Keys must stay in sync with BannedWords and BannedPhrases.
var StyleReplacements = map[string]string{
	// words
	"delve": "examine", "underscore": "highlight", "testament": "sign",
	"foster": "encourage", "maximize": "increase", "navigate": "handle",
	"tapestry": "mix", "catalyst": "trigger", "elevate": "raise",
	"paradigm": "model", "revolutionize": "transform", "paramount": "essential",
	"leverage": "use", "harness": "use", "unlock": "enable", "seamless": "smooth",
	"robust": "strong", "realm": "area", "landscape": "field", "beacon": "example",
	"game-changer": "breakthrough", "cutting-edge": "advanced", "utilize": "use",
	"myriad": "many", "vibrant": "lively", "bustling": "busy",
	"whimsical": "playful", "profound": "deep",
	// phrases
	"in today's fast-paced world": "today", "at its core": "essentially",
	"it's important to note": "note", "furthermore": "also", "moreover": "also",
	"in conclusion": "finally", "in summary": "overall", "let's face it": "admittedly",
	"but here's the kicker": "but", "when it comes to": "for", "in the realm of": "in",
	"look no further": "stop", "rest assured": "certainly", "needless to say": "clearly",
	"not only": "not just", "but also": "but", "whether you're": "whether you are",
	"that said": "still", "in essence": "essentially", "ultimately": "finally",
	"paradigm shift": "shift", "the dawn of": "the start of", "in today's world": "today",
}

// MetricTerms are evaluation metrics that must never appear in a draft unless a
// verified claim also uses them, guarding against silent metric conversion.
var MetricTerms = []string{
	"perplexity", "ppl", "cross-entropy", "cross entropy", "log-likelihood",
	"log likelihood", "bleu", "rouge", "f1", "wer", "cer",
	"bits per byte", "bits-per-byte",
}

// AssertiveVerbs flag sentences that state a result as settled fact; combined
// with a hedged claim they signal a possible hedge upgrade.
var AssertiveVerbs = []string{
	"demonstrates", "demonstrated", "proves", "proven", "proved",
	"establishes", "established", "confirms", "confirmed", "guarantees",
	"guaranteed", "ensures", "ensured", "shows that", "showed that",
}

// ClaimTypes and ClaimStrengths are the accepted enumerations for a claim
// record. Anything outside them is dropped during verification.
var (
	ClaimTypes = map[string]bool{
		"metric": true, "mechanism": true, "definition": true,
		"method": true, "result": true, "limitation": true,
	}
	ClaimStrengths = map[string]bool{
		"demonstrated": true, "hedged": true, "speculation-or-future-work": true,
	}
	HedgeStrengths = map[string]bool{
		"hedged": true, "speculation-or-future-work": true,
	}
)

// WriterStopwords are common words ignored when measuring token overlap between
// a draft sentence and a claim, so overlap reflects meaningful shared terms.
var WriterStopwords = map[string]bool{
	"about": true, "above": true, "after": true, "again": true, "against": true,
	"along": true, "among": true, "around": true, "because": true, "before": true,
	"being": true, "below": true, "between": true, "beyond": true, "could": true,
	"does": true, "doing": true, "during": true, "either": true, "every": true,
	"framework": true, "further": true, "hence": true, "however": true, "into": true,
	"itself": true, "loops": true, "might": true, "other": true, "process": true,
	"rather": true, "result": true, "results": true, "since": true, "some": true,
	"such": true, "system": true, "than": true, "that": true, "their": true,
	"them": true, "then": true, "there": true, "these": true, "they": true,
	"those": true, "through": true, "under": true, "until": true, "using": true,
	"value": true, "when": true, "where": true, "which": true, "while": true,
	"with": true, "within": true, "would": true,
}
