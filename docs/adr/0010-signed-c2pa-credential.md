# 0010. Sign the C2PA manifest, and emit a portable verification record

- **Status:** Accepted
- **Date:** 2026-09-11

## Context

[0008](0008-sentence-attribution-c2pa.md) emits a C2PA manifest *definition*
and `draft --verify` recomputes digests. For regulatory or enterprise use the
provenance claim needs to be a *signed* Content Credential, and a verifier
needs an outcome it can consume without the CLI — while the tool's keyless,
local-first promise must not be compromised.

## Decision

When `DRAFT_C2PA_CERT` / `DRAFT_C2PA_KEY` are set and `c2patool` is installed,
`draft` signs the manifest into a detached `.c2pa` credential bound to the body,
and `draft --verify` validates the signature and trust chain in addition to the
digests. `draft --verify --json` emits a stable `draft.verification-record/v1`
whose schema lives in the importable `provenance` package. Signing is opt-in;
with no certificate the manifest stays an unsigned definition, exactly as before.

## Alternatives considered

- **Bind `c2pa-rs` / a Go signer directly.** Rejected: shelling to `c2patool`
  keeps the crypto and its trust store out of the binary and off the default
  path, matching [0003](0003-session-cli-backends.md)'s no-embedded-secrets
  stance. C2PA does not natively embed in Markdown anyway, so a sidecar is the
  shape regardless.
- **Trust the tool's exit status.** Rejected: `c2patool` reports a text asset
  as "unsupported file type" while still writing a valid sidecar, so signing
  confirms success by *verifying the produced credential*, not by exit code.
- **Fail verification on an untrusted certificate.** Rejected: a development
  certificate signs a genuinely valid credential; it is reported as
  valid-but-untrusted, and only a broken signature fails.

## Consequences

- Signing needs an external tool and a certificate; production key custody
  (KMS/HSM) is the operator's, deliberately outside this repository.
- The record schema is a compatibility contract: changing it is a versioned,
  breaking event, hence the `/v1`.
- Everything stays keyless and local unless the operator opts in.
