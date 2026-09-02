# Packaging templates

A starting point per format. These are offered as a convenience and are **not
authoritative** over your distribution's own conventions — if yours disagrees,
yours is right.

Read [`docs/packaging.md`](../docs/packaging.md) first: it holds the licence
grant, the minimum-toolchain policy, the offline build and test instructions,
and how to verify a release signature.

| Directory                | Format                 | Generated or template                                                                                                  |
| ------------------------ | ---------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| —                        | `.deb`, `.rpm`, `.apk` | **Generated** at release time by GoReleaser (`nfpms:` in `.goreleaser.yaml`) from the same binaries the archives carry |
| [`aur/`](aur/)           | Arch `PKGBUILD`        | Template                                                                                                               |
| [`brew/`](brew/)         | Homebrew formula       | Template                                                                                                               |
| [`nix/`](nix/)           | Nix flake              | Template                                                                                                               |
| [`docker/`](docker/)     | Container image        | Template                                                                                                               |
| [`VERIFY.md`](VERIFY.md) | —                      | How to check what you are packaging                                                                                    |

Native packages are generated rather than templated on purpose: a `.deb` and a
`.tar.gz` of the same version are then built from one set of binaries and
cannot describe different software.

## Not yet submitted anywhere

`draft` is not currently in any distribution. Nothing here has been filed with
Debian, Fedora, nixpkgs, the AUR or Homebrew core. The templates exist so that
whoever does it first — maintainer or contributor — is not starting from
nothing.

A [Repology](https://repology.org) badge belongs in the README once at least
two distributions track the package, and not before: a badge showing "not
packaged anywhere" is worse than no badge.
