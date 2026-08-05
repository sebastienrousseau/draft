# Release security

Release signing is isolated in the protected GitHub `release` environment.
Only `v*` tag deployments are allowed, and `sebastienrousseau` must approve a
deployment before its secrets are exposed to a runner.

## Credentials

The environment stores the Developer ID Application and Installer PKCS#12
files, their generated passwords, and a Developer-role App Store Connect team
key. This project uses the API key only for notarization requests; never place its
private key or a PKCS#12 file in the repository, logs, artifacts, or caches.

Public certificates live in `build/apple/` solely for expiry monitoring. The
weekly `release-security` workflow opens issues at 60 and 30 days before a
certificate expires and when the API key reaches 80 days of age.

## Rotation

1. Generate the replacement credential in Apple Developer or App Store Connect.
2. Test it locally with `notarytool history` or a disposable notarization.
3. Replace the corresponding protected environment secret.
4. Replace the public certificate in `build/apple/`, when applicable.
5. Update `APPLE_API_KEY_ROTATED_AT` for an API-key rotation.
6. Run `release-security` manually and close the resolved reminder issue.
7. Revoke the previous credential only after the new one passes.

## Emergency revocation

If a signing key may be exposed, stop releases, revoke the affected certificate
in Apple Developer or API key in App Store Connect, remove it from the GitHub
environment, and rotate it immediately. Review release tags, Actions logs,
environment deployments, and Apple notarization history for unexpected use.
Publish a replacement release only after both GitHub and Apple audit trails are
clean.

## Release verification

The release workflow signs and notarizes both macOS binaries, creates a signed
universal installer package, notarizes and staples the package, refreshes the
Sigstore-signed checksum manifest, publishes provenance attestations, and then
downloads every asset on a clean macOS runner for independent verification.
