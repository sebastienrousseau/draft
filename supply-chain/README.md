# Supply chain

What is depended on, how it is pinned, and what is checked on every push.

## The dependency graph is deliberately tiny

Three direct dependencies, all from one family:

```text
github.com/charmbracelet/bubbletea    terminal UI runtime
github.com/charmbracelet/bubbles      terminal UI widgets
github.com/charmbracelet/lipgloss     terminal styling
```

All three are used **only by the dashboard**. The library — extraction,
grounding, validation, frontmatter — depends on nothing outside the standard
library, so importing `draft` as a library pulls in no third-party code on any
path that touches a claim.

Everything else in `go.sum` is transitive from those three.

## How it is pinned

| Mechanism                           | What it guarantees                                  |
| ----------------------------------- | --------------------------------------------------- |
| `go.sum`, committed                 | Every module version resolves to a known hash       |
| `-mod=readonly` (Go's default)      | A build cannot silently acquire a different version |
| `go mod verify` in CI               | The module cache matches those hashes               |
| GitHub Actions pinned by commit SHA | A tag cannot be repointed at different code         |
| `GOFLAGS` unset in CI               | No local override changes resolution                |

There is no vendor directory. `go mod vendor` produces one deterministically
for build environments that need it; see [`docs/packaging.md`](../docs/packaging.md).

## What runs on every push

| Check                                   | Tool              |
| --------------------------------------- | ----------------- |
| Known vulnerabilities, call-graph aware | `govulncheck`     |
| Static analysis for injected patterns   | CodeQL            |
| Module hash integrity                   | `go mod verify`   |
| Licence machine-readability             | `reuse lint`      |
| Repository posture                      | OpenSSF Scorecard |

`govulncheck` is call-graph aware, so it reports only advisories on code paths
this module actually reaches — the difference between an actionable list and a
wall of noise nobody reads.

## Provenance of what ships

Every release carries a CycloneDX SBOM per archive, a Sigstore-signed checksum
file, and a SLSA build-provenance attestation. Verification commands are in
[`pkg/VERIFY.md`](../pkg/VERIFY.md).

There is no `KEYS.asc`: signing is **keyless**, through Sigstore and the
workflow's OIDC identity. There is no long-lived private key to publish, lose
or rotate, and verification checks the identity of the workflow that built the
artefact rather than possession of a key.

## Adding a dependency

The bar is high, and deliberately so. Before adding one, answer in the pull
request:

1. What does it do that the standard library cannot, in a reasonable amount of
   code?
2. Does anything on the grounding path need it? If yes, expect a no — that
   path's dependency-free status is a security property, not an accident.
3. What is its own dependency graph?
4. What happens to `draft` if it is abandoned?

## Not yet done

- **A CII / OpenSSF Best Practices self-assessment.** It requires registering
  the project at <https://bestpractices.dev>, which is a maintainer action.
- **Reproducible builds.** Not verified here, and therefore not claimed. See
  [`pkg/VERIFY.md`](../pkg/VERIFY.md).
