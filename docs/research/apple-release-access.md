# Apple release access

> **Closed as unnecessary:** ADR 0006 selects npm as the initial distribution
> channel. Apple membership, Developer ID signing, and notarization are no
> longer prerequisites for Skillet's first public release.

Checked 2026-07-10 for [Verify Apple signing and notarization
access](https://github.com/jnyross/Skill-Manager/issues/40).

## Current status

Skillet is not ready to sign or notarize a public macOS Release.

- Apple Command Line Tools are installed at
  `/Library/Developer/CommandLineTools`.
- `notarytool` 1.1.0 and `/usr/bin/codesign` are available.
- The login keychain has one valid Apple Configurator identity and no
  **Developer ID Application** identity. Apple Configurator identity is not a
  substitute for Developer ID distribution signing.
- The GitHub repository exposes no Actions secret names and has no configured
  release environment. Default workflow permissions are read-only.
- The signed-in Apple Developer account was checked in Chrome on 2026-07-10.
  Its account page displayed **Join the Apple Developer Program**, so it has no
  active paid membership. Enrollment is required before certificate
  provisioning.

## Provisioning checklist

This checklist requires the maintainer's authority. Do not create accounts,
purchase membership, issue certificates, or write secrets without explicit
approval.

1. Confirm that the publishing Apple account has active Apple Developer Program
   membership and record its Team ID. Apple requires membership to obtain
   Developer ID certificates ([Developer ID](https://developer.apple.com/developer-id/)).
2. From the Apple Developer account, create a **Developer ID Application**
   certificate for the publishing team. Generate its private key in a controlled
   keychain, export certificate plus private key as a password-protected PKCS#12
   file, and keep the original and password outside Git and chat.
3. Create the least-privilege notarization credential supported by Apple's
   current notary service. Prefer an App Store Connect API key usable with
   `notarytool` over a personal Apple ID password; retain only the key ID, issuer
   ID, Team ID, and private `.p8` material required by the release job
   ([customizing notarization](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow)).
4. Create a protected GitHub Actions environment named `release`. Require the
   maintainer as reviewer and restrict deployment to protected `v*` tags.
5. Store the PKCS#12 bytes, its password, the notarization private key, key ID,
   issuer ID, and Team ID as environment-scoped secrets. Secret names may be
   stable workflow inputs; values must never be printed, uploaded as artifacts,
   or exposed to pull-request jobs.
6. Make the release job create a temporary keychain, import the Developer ID
   identity, sign both Mach-O binaries with hardened runtime and secure
   timestamp, submit the final container with `notarytool`, wait for acceptance,
   verify signature and Gatekeeper assessment, and delete the temporary
   keychain on every exit path. Apple lists Developer ID signing, hardened
   runtime, timestamp, and notarization requirements in
   [Resolving common notarization issues](https://developer.apple.com/documentation/security/resolving-common-notarization-issues).
7. Before enabling a public release, prove a deliberately quarantined download
   on clean supported Intel and Apple Silicon Macs. A successful API submission
   alone is not the user-facing trust path.
8. Record certificate expiry and credential-owner recovery in the release
   runbook. Rotate before expiry through a separately reviewed change; never
   overwrite evidence for a previously published Release.

## Ready evidence

The provisioning task is complete only when all of these are true without
revealing secret values:

- `security find-identity -v -p codesigning` lists a valid Developer ID
  Application identity in the controlled release context;
- a throwaway signed CLI passes `codesign --verify --strict --verbose=2`;
- `notarytool submit --wait` accepts a throwaway distribution container;
- the protected `release` environment and required secret **names** are visible
  in GitHub configuration;
- an untrusted pull-request workflow cannot read signing secrets or obtain
  release write permission; and
- the temporary keychain and extracted credentials are absent after the job.
