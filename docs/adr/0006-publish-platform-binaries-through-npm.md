# Publish platform binaries through npm

## Status

Accepted

## Context

Skillet is a Go command-line application that must be easy to install and
upgrade on supported macOS and Linux machines without requiring Go or a source
checkout. The previously selected GitHub archive, Homebrew tap, and direct
installer design made Apple Developer Program membership, Developer ID signing,
and notarization release blockers. Apple rejected enrollment through both its
website and the official Developer app for the maintainer's account.

The intended audience for Skillet already uses command-line agent tooling, for
which Node.js and npm are a practical shared distribution substrate. npm can
select dependencies by operating system and CPU, expose a command through the
`bin` field, and publish from GitHub Actions through short-lived trusted
publishing credentials with provenance.

## Decision

Publish Skillet under the scoped public package `@jnyross/skillet`. The user
contract is:

```sh
npm install --global @jnyross/skillet
npm install --global @jnyross/skillet@latest
```

The first command installs Skillet and the second is the explicit same-channel
upgrade command.

Use one small JavaScript launcher package plus four version-locked optional
platform packages containing the Go executables for Darwin and Linux on arm64
and amd64. Each platform package declares its exact `os` and `cpu`; the launcher
selects only the executable for the current supported pair and fails clearly if
the required optional package is absent. All five packages share one version.

Publish from a protected GitHub Actions release workflow using npm trusted
publishing and provenance. Do not store a long-lived npm publish token in the
repository. Preserve checksums and target-native smoke tests in the release
workflow even though npm verifies package integrity in transit.

Apple Developer Program membership, Developer ID signing, notarization,
Homebrew, and the first-party shell installer are no longer launch blockers.
They may be added later as secondary channels, but must not complicate the
initial npm release.

## Consequences

- Installation and upgrades require a supported Node.js/npm installation.
- The unscoped `skillet` npm name is already occupied, so documentation must use
  the full `@jnyross/skillet` package name.
- The repository must become public before the provenance-backed release.
- The maintainer needs an npm account that owns the `@jnyross` scope and must
  configure the package's trusted GitHub publisher once.
- macOS users receive an unsigned Go executable through npm. The release
  verification ticket must test the real npm install-and-run path on clean
  supported Macs rather than infer readiness from build success.
