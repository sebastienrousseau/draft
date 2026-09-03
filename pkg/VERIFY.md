# Verifying what you are packaging

Before building a package, check that the source or archive you have is the one
upstream published. Every release carries the material to do this.

## An archive

```console
VERSION=0.0.32

# 1. The checksum file is signed with keyless Sigstore.
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/sebastienrousseau/draft/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# 2. Verifying the checksum file transitively covers every artefact in it.
sha256sum -c checksums.txt --ignore-missing

# 3. Build provenance: which workflow, which commit, which runner.
gh attestation verify "draft_${VERSION}_linux_amd64.tar.gz" \
  -R sebastienrousseau/draft
```

## The dependency graph

A CycloneDX SBOM ships beside each archive as `<archive>.sbom.json`:

```console
grype sbom:draft_${VERSION}_linux_amd64.tar.gz.sbom.json   # or your scanner
```

## macOS binaries

Signed with a Developer ID certificate and notarized by Apple:

```console
codesign --verify --deep --strict --verbose=2 draft
spctl --assess --type execute --verbose draft
```

## Building from a tag

```console
git clone https://github.com/sebastienrousseau/draft
cd draft && git checkout v${VERSION}
git verify-commit HEAD    # commits are signed
go test ./...
```

## What is not claimed

**Reproducible builds.** Two builds of the same tag have not been verified to
produce identical binaries here. This is not a claim that it is impossible —
`CGO_ENABLED=0` with `-trimpath` gets most of the way — only that it has not
been demonstrated, and an unverified claim is worse than none. If you verify it
in your build environment, please open an issue saying so.

**Any support window.** The supported version is the latest release. See
[SUPPORT.md](../SUPPORT.md).
