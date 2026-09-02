# Packaging `draft`

For distribution maintainers. Everything here is a commitment, not a
suggestion: if any of it is wrong or inconvenient for your ecosystem, open an
issue and it will be treated as a bug.

## Contents

- [At a glance](#at-a-glance)
- [Licensing](#licensing)
- [Minimum toolchain](#minimum-toolchain)
- [Dependencies and pinning](#dependencies-and-pinning)
- [Building offline](#building-offline)
- [Runtime dependencies](#runtime-dependencies)
- [What to install](#what-to-install)
- [Verifying a release](#verifying-a-release)
- [Version and release cadence](#version-and-release-cadence)

## At a glance

|                 |                                                              |
| --------------- | ------------------------------------------------------------ |
| Upstream        | <https://github.com/sebastienrousseau/draft>                 |
| Licence         | `MIT OR Apache-2.0` (SPDX, REUSE-compliant)                  |
| Language        | Go, `CGO_ENABLED=0` — static, no shared-library dependencies |
| Minimum Go      | read from `go.mod`; do not hardcode                          |
| Build           | `go build ./cmd/draft`                                       |
| Binary          | `draft`                                                      |
| Runtime deps    | `poppler-utils` (recommended), none required                 |
| Networked build | no, if the module cache is vendored                          |

## Licensing

Dual-licensed **MIT OR Apache-2.0** at your option. Both texts ship as
`LICENSE-MIT` and `LICENSE-APACHE`. Every file carries machine-readable SPDX
headers and the repository passes `reuse lint` in CI, so
`reuse spdx` will give you a complete manifest without manual auditing.

## Minimum toolchain

The floor is declared once, in `go.mod`, and is built and tested by a dedicated
CI job on every pull request. **Read it from there** rather than copying a
number into a spec file that will drift.

Policy for when it rises, and why no distro-compatibility claim is made, is in
the README under [Minimum Go policy](../README.md#minimum-go-policy). Briefly:
it rises only for a concrete need, as a minor release, listed in the changelog.

## Dependencies and pinning

Three direct dependencies, all from the Charm terminal-UI family:

```text
github.com/charmbracelet/bubbletea
github.com/charmbracelet/bubbles
github.com/charmbracelet/lipgloss
```

`go.sum` is committed and covers the full graph. Go's default build mode is
`-mod=readonly`, so a build cannot silently acquire a different version; pass
`-mod=mod` only if you intend that.

There is no vendor directory in the repository. If your build environment
requires one, `go mod vendor` produces it deterministically — see below.

## Building offline

```console
# Once, with network:
go mod vendor

# Thereafter, with none:
go build -mod=vendor -trimpath \
  -ldflags "-s -w -X main.version=$VERSION" \
  -o draft ./cmd/draft
```

`-trimpath` is recommended: it removes local filesystem paths from the binary,
which is both smaller and closer to reproducible.

Tests run offline too. `go test ./...` needs no network; the only tests that
touch external tools skip themselves when those tools are absent:

```console
go test ./...          # skips PDF tests if pdftotext is missing
```

## Runtime dependencies

| Tool                        | Needed for   | Suggested relationship                                                                                                         |
| --------------------------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------ |
| `pdftotext` (poppler-utils) | PDF sources  | **Recommends** / **Suggests** — the tool runs without it and reports the gap via `draft --doctor`, but PDFs are its main input |
| `textutil`                  | DOCX sources | macOS only; not applicable to Linux packages                                                                                   |

Neither is a hard requirement: `draft` reports a missing tool with an
actionable message rather than failing at load, and Markdown and text sources
work with no external tools at all.

## What to install

```text
bin/draft                                  the binary
share/man/man1/draft.1                     generated manpage
share/bash-completion/completions/draft    generated
share/zsh/site-functions/_draft            generated
share/fish/vendor_completions.d/draft.fish generated
share/doc/draft/{README.md,CHANGELOG.md}   documentation
share/licenses/draft/LICENSE-{MIT,APACHE}  licence texts
```

`GNUmakefile` implements exactly this and honours the usual conventions, so the
simplest correct packaging step is:

```console
make -f GNUmakefile install PREFIX=/usr DESTDIR="$pkgdir"
```

Manpages and completions are **generated from the CLI definitions** at build
time, never committed, so they cannot drift from `--help`.

## Verifying a release

Every release carries a checksum file signed with keyless Sigstore, a CycloneDX
SBOM per archive, and a SLSA build-provenance attestation. macOS binaries are
Developer ID signed and notarized.

```console
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/sebastienrousseau/draft/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

sha256sum -c checksums.txt --ignore-missing
gh attestation verify draft_${VERSION}_linux_amd64.tar.gz -R sebastienrousseau/draft
```

Full detail, including the release environment's protections, is in
[`RELEASE_SECURITY.md`](RELEASE_SECURITY.md).

**No reproducible-build claim is made.** Not because it is thought impossible,
but because it has not been verified here, and an unverified claim is worse
than none. If you verify it in your build environment, that result is welcome.

## Version and release cadence

Versions are `0.0.x` while the API stabilises. What counts as a breaking change
— including the rule that a change to *generated output* is breaking even when
no signature moves — is in the README under
[Stability guarantees](../README.md#stability-guarantees).

Releases are tag-triggered and automated; there is no fixed cadence.

## Packaging templates

`pkg/` holds a starting point per format, generated from the same release
build. They are offered as a convenience and are not authoritative over your
distribution's own conventions.
