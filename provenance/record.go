// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package provenance

// RecordKind identifies the verification-record schema and version. It is a
// stable contract: anything that consumes a record — CI, another tool, a hosted
// verifier — keys on it, so it changes only when the schema does.
const RecordKind = "draft.verification-record/v1"

// VerificationRecord is the portable, machine-readable outcome of checking an
// article against the provenance beside it — the same thing `draft --verify`
// reports for a human, in a form a program can consume and store.
//
// It is deliberately self-contained and transport-agnostic: it names what was
// checked (digests, grounding, sources, signature) and the verdict, without
// reference to where the files live. That is what lets verification happen
// anywhere — a receipt travels with the artifact, and re-verifying it needs
// only the record and (for the digests) the article. The type lives in this
// importable package so a separate verifier can depend on the schema without
// pulling in the CLI.
type VerificationRecord struct {
	Kind      string          `json:"kind"`
	Verified  bool            `json:"verified"`
	Generator Generator       `json:"generator"`
	Article   ArticleState    `json:"article"`
	Grounding GroundingState  `json:"grounding"`
	Ledger    *DigestState    `json:"ledger,omitempty"`
	Sources   []SourceState   `json:"sources,omitempty"`
	Signature *SignatureState `json:"signature,omitempty"`
	Problems  []string        `json:"problems,omitempty"`
}

// ArticleState is the body digest and whether it still matches the manifest.
type ArticleState struct {
	SHA256  string `json:"sha256"`
	Matches bool   `json:"matches"`
}

// GroundingState is the grounding summary the manifest recorded.
type GroundingState struct {
	Claims                    int `json:"claims"`
	Sentences                 int `json:"sentences"`
	Attributed                int `json:"attributed"`
	UngroundedNumberSentences int `json:"ungrounded_number_sentences"`
}

// DigestState is the outcome of a digest check that is only made when the file
// is present (the ledger).
type DigestState struct {
	Checked bool `json:"checked"`
	Matches bool `json:"matches"`
}

// SourceState is one source's identity and whether it was found and unchanged.
type SourceState struct {
	Path    string `json:"path"`
	Found   bool   `json:"found"`
	Matches bool   `json:"matches"`
}

// SignatureState is the outcome of validating a detached C2PA credential. It is
// a plain value so this package needs no dependency on the signing tool: the
// caller fills it from whatever verified the credential. Present is false when
// no credential accompanied the set.
type SignatureState struct {
	Present bool `json:"present"`
	// Checked is false when a credential was present but could not be validated
	// here (for example c2patool is not installed). An unchecked credential does
	// not fail the record — a verifier that can check it would — but it is
	// reported so nothing is silently ignored.
	Checked bool   `json:"checked"`
	Valid   bool   `json:"valid"`
	Trusted bool   `json:"trusted"`
	State   string `json:"state,omitempty"`
}

// NewVerificationRecord builds a record from a CheckReport and an optional
// signature outcome. The overall verdict is the report's own verdict AND, when
// a credential was checked, a valid signature — a valid-but-untrusted
// credential (a development certificate) does not fail the record, matching
// `draft --verify`.
func NewVerificationRecord(r CheckReport, sig *SignatureState) VerificationRecord {
	rec := VerificationRecord{
		Kind:      RecordKind,
		Generator: r.Generator,
		Article:   ArticleState{SHA256: r.BodySHA256, Matches: r.DigestMatches},
		Grounding: GroundingState{
			Claims:                    len(r.Grounding.Claims),
			Sentences:                 r.Grounding.Sentences,
			Attributed:                r.Grounding.Attributed,
			UngroundedNumberSentences: r.Grounding.SentencesWithUngroundedNumbers,
		},
		Problems: r.Problems,
	}
	if r.LedgerChecked {
		rec.Ledger = &DigestState{Checked: true, Matches: r.LedgerMatches}
	}
	for _, s := range r.Sources {
		rec.Sources = append(rec.Sources, SourceState(s))
	}
	rec.Signature = sig
	// A signature fails the verdict only when it was actually checked and found
	// invalid; a missing credential, or one that could not be checked here, does
	// not — matching `draft --verify`.
	rec.Verified = r.OK() && (sig == nil || !sig.Checked || sig.Valid)
	return rec
}
