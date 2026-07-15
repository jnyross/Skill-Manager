# Public macOS and Linux distribution requirements

> **Superseded distribution decision:** ADR 0006 replaces the GitHub archive,
> Homebrew, direct-installer, and Apple-signing launch path below with a scoped
> npm package and platform-specific binary packages. This document remains the
> evidence trail for that earlier route; its Apple requirements are not current
> release blockers.

Researched 2026-07-10 for [Research trusted macOS and Linux distribution
requirements](https://github.com/jnyross/Skill-Manager/issues/33).

## Decision

Distribute Skillet as four versioned, prebuilt archives attached to immutable
GitHub Releases:

- `skillet_<version>_darwin_amd64.tar.gz`
- `skillet_<version>_darwin_arm64.tar.gz`
- `skillet_<version>_linux_amd64.tar.gz`
- `skillet_<version>_linux_arm64.tar.gz`

Each release must also contain one `checksums.txt` covering the exact bytes of
every archive. Build with `CGO_ENABLED=0`, test each target, sign and notarize
both macOS binaries, and generate GitHub artifact attestations for the release
archives. Offer the same artifacts through:

1. a first-party `homebrew-tap`, with `brew upgrade skillet` as its upgrade
   path; and
2. a first-party POSIX installer, with rerunning the installer as its upgrade
   path.

This is the smallest distribution matrix that covers the stated public macOS
and Linux destination on the CPU architectures supported by both Go and
current Homebrew. It intentionally does not claim Windows, 32-bit machines,
native Linux packages, an app bundle, or self-update support.

## Requirements

These are release blockers, either imposed by a selected platform or necessary
to make the advertised install and upgrade contract true.

### Artifact and runtime contract

1. Produce separate `darwin/amd64`, `darwin/arm64`, `linux/amd64`, and
   `linux/arm64` artifacts. Go lists all four as valid targets, while Homebrew's
   supported CPU set is 64-bit Intel and ARM64/AArch64
   ([Go environment targets](https://go.dev/doc/install/source#environment),
   [Homebrew support tiers](https://docs.brew.sh/Support-Tiers)). Do not silently
   serve an x86 binary under Rosetta or an emulator when a native target is in
   scope.
2. Build releases from the tagged commit with the repository's declared Go
   version, `CGO_ENABLED=0`, and the default `GOAMD64=v1` baseline unless a
   later ticket deliberately raises it. The Go command defines `GOAMD64=v1` as
   the default and exposes `CGO_ENABLED` as the switch controlling cgo
   ([Go command environment](https://pkg.go.dev/cmd/go#hdr-Environment_variables)).
3. Declare the actual OS floor in public installation documentation. With the
   repository's current Go 1.24 line, Go requires Linux kernel 3.2 or later and
   Go 1.24 is the last release supporting macOS 11; newer Go toolchains can
   raise that floor
   ([Go 1.24 release notes](https://go.dev/doc/go1.24#ports),
   [Go minimum requirements](https://go.dev/wiki/MinimumRequirements)). A
   release must therefore re-check its produced Mach-O deployment target and
   current Go minimums instead of promising a timeless OS version.
4. Package only the executable plus the minimum user-facing license/readme
   material chosen by the licensing ticket. Archive paths and filenames must be
   deterministic and stable because the installer and tap consume them.
5. Generate SHA-256 for the final uploaded archive bytes, not for an
   intermediate unsigned binary. Homebrew formula resources require a specific
   URL and SHA-256, and Homebrew uses the changed URL/checksum as the normal
   version update
   ([Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)).

Local evidence: on 2026-07-10 this tree built all four targets successfully
with `CGO_ENABLED=0`. The two Linux executables were reported by `file` as
statically linked. The arm64 macOS executable linked only Apple system
libraries and carried a macOS 12.0 deployment target. That proves the current
dependency graph can produce the matrix; it does not replace target-native
release tests or establish future compatibility.

### macOS signing and notarization

1. Sign each Mach-O executable with a valid **Developer ID Application**
   certificate before archiving it. Apple says command-line tools use that
   certificate and that software distributed outside the Mac App Store should
   be signed so Gatekeeper can identify the developer
   ([resolving notarization issues](https://developer.apple.com/documentation/security/resolving-common-notarization-issues),
   [Developer ID](https://developer.apple.com/developer-id/)).
2. Submit the signed deliverable to Apple's notary service with `notarytool`
   (or its Notary API), wait for acceptance, and validate the downloaded
   release artifact on a clean supported Mac. Apple no longer accepts
   `altool`, and says directly distributed software should be notarized
   ([notarization overview](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution),
   [Developer ID support](https://developer.apple.com/support/developer-id/)).
3. Keep the Developer ID private key and Apple notarization credentials out of
   the repository and expose them only to the release job that needs them. An
   Apple Developer Program membership and Developer ID certificate are
   prerequisites for this trust path
   ([Developer ID support](https://developer.apple.com/support/developer-id/)).
4. Verify the final artifact, not merely the pre-upload binary: check the code
   signature, assess it with Gatekeeper, and run it after a browser-style
   quarantined download. A checksum proves byte equality; it does not prove
   Gatekeeper acceptance.

For a bare command-line archive, notarization ticket stapling may not be
available in the same way as for an app, disk image, or installer package.
Apple publishes the ticket online for Gatekeeper, so online notarization plus a
clean-machine Gatekeeper test is the minimum accepted route. Whether Skillet
should instead ship a notarizable/stapleable signed `.pkg` or disk image is an
implementation question, not a requirement for the initial tarball route.

### GitHub release and workflow boundary

1. Publish releases from immutable version tags and enable GitHub immutable
   releases before the first public binary release. Immutable releases lock the
   tag and assets and automatically create a release attestation; GitHub
   recommends creating a draft, attaching all assets, then publishing it
   ([immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases),
   [preventing release changes](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/establish-provenance-and-integrity/preventing-changes-to-your-releases)).
2. Build from the release tag and upload explicit assets. GitHub's automatic
   source ZIP and tarball are generated from repository content and are not the
   platform binaries
   ([about releases](https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases)).
3. Give each Actions job only its necessary `GITHUB_TOKEN` permissions. A build
   job needs `contents: read`; a job creating or uploading a release needs
   `contents: write`; provenance generation needs `id-token: write`,
   `attestations: write`, and `contents: read`. Unspecified permissions become
   `none`
   ([workflow permissions](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#permissions),
   [artifact attestations](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations)).
4. Pin every third-party and GitHub-authored action to a reviewed full commit
   SHA. GitHub states that a full SHA is the only immutable way to consume an
   action
   ([secure use reference](https://docs.github.com/en/actions/reference/security/secure-use#using-third-party-actions)).
5. Do not expose release credentials to pull requests. PR validation must run
   without Apple secrets and without release write permission; signing and
   publishing run only from a protected tag/release path after CI succeeds.
6. Generate a build-provenance attestation for every final archive and document
   `gh attestation verify <archive> --repo jnyross/Skill-Manager`, preferably
   constrained further with `--signer-workflow`. GitHub documents both the
   required permissions and identity checks
   ([artifact attestations](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations),
   [`gh attestation verify`](https://cli.github.com/manual/gh_attestation_verify)).
7. Retain `checksums.txt` even with attestations. SHA-256 is universally usable
   by the installer and Homebrew, while attestation verification gives stronger
   provenance to users who have GitHub CLI.

### Homebrew channel

1. Use a public repository named `jnyross/homebrew-tap`, which enables the
   short tap name and the user command `brew install jnyross/tap/skillet`.
   Homebrew documents taps as Git repositories and recommends the
   `homebrew-` prefix for GitHub-hosted taps
   ([creating a tap](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap)).
2. Maintain a stable `Formula/skillet.rb` containing `desc`, `homepage`,
   `version` when URL inference is insufficient, a platform/architecture-
   specific immutable release URL, its SHA-256, the selected license, an
   install step that places `skillet` in `bin`, and a real `test do` block.
3. Select the correct archive explicitly for macOS/Linux and Intel/ARM64 and
   fail unsupported combinations. Homebrew supports platform conditionals and
   architecture inspection, and its current Tier 1 set includes both CPU
   families on macOS and Linux
   ([Formula Cookbook](https://docs.brew.sh/Formula-Cookbook),
   [Homebrew support tiers](https://docs.brew.sh/Support-Tiers)).
4. Test the formula on all four advertised targets before updating the tap.
   Homebrew considers third-party tap software unsupported by Homebrew itself,
   so Skillet owns that compatibility promise
   ([Homebrew support tiers](https://docs.brew.sh/Support-Tiers#unsupported-software)).
5. Update the formula only after the matching immutable GitHub Release exists;
   change version, URLs, and checksums together. Users then upgrade through
   normal `brew update`/`brew upgrade skillet` behavior. Never make the formula
   invoke Skillet's direct installer or fetch an unversioned executable.
6. Start with a project-owned tap rather than `homebrew/core`. Core generally
   dislikes binary formulae, requires stable tagged releases, and applies
   notability thresholds that a new public project is unlikely to meet
   ([acceptable formulae](https://docs.brew.sh/Acceptable-Formulae)).

### First-party installer channel

1. Publish the installer source in the public repository over HTTPS. The
   one-line route may be `curl -fsSL <fixed HTTPS installer URL> | sh`, but the
   README must also offer a download-inspect-run route; piping a script executes
   it before any release-archive checksum can protect the user.
2. Write the installer for POSIX `sh` unless it deliberately checks for and
   invokes another shell. Use strict failure handling, quote every path, create
   a private temporary directory, and register cleanup for normal exit and
   signals. Do not require Go, Git, Homebrew, root, or a package manager.
3. Detect only supported `uname -s`/`uname -m` pairs, normalize them to the four
   artifact names, and stop with a useful error for everything else. Never
   guess an artifact or fall back to executing a source archive.
4. Resolve `latest` to a concrete stable version, then download both that
   version's archive and checksum manifest from immutable release URLs. GitHub
   supports `/releases/latest` and `/releases/latest/download/<asset>`, but the
   installer should print and use the resolved version so the operation is
   auditable
   ([linking to releases](https://docs.github.com/en/repositories/releasing-projects-on-github/linking-to-releases)).
5. Require HTTPS, follow redirects deliberately, fail on HTTP errors, and
   propagate download failures. Verify the selected archive against its exact
   SHA-256 entry **before** extraction or execution. Support the native checksum
   tool available on the target (`shasum -a 256` on macOS; `sha256sum` on most
   Linux systems) and fail if neither exists.
6. Reject archive entries that are absolute or traverse outside the temporary
   directory before extraction. Verify that exactly the expected executable is
   present, then install it atomically with executable permissions.
7. Default to a user-writable destination already on `PATH`; otherwise use
   `${XDG_BIN_HOME:-$HOME/.local/bin}` and print the exact PATH change the user
   must make. Accept an explicit destination environment variable. Never edit
   shell startup files silently and never invoke `sudo` itself.
8. Before overwriting, determine whether the existing `skillet` is managed by
   Homebrew. If so, stop and direct the user to `brew upgrade skillet`. For a
   direct install, replace only after verification and preserve/restore the old
   executable if the atomic replacement fails.
9. Print the installed version, destination, source release URL, and the command
   for the same-channel upgrade. Provide an idempotent `--version <semver>` path
   so CI and users can pin installs instead of being forced onto `latest`.

These installer controls are Skillet's own security contract rather than rules
imposed by GitHub or Go. Homebrew's first-party installer is useful precedent:
its documentation emphasizes supported prefixes, explaining actions before
performing them, confirmation, and an explicit noninteractive mode
([Homebrew installation](https://docs.brew.sh/Installation)).

### Private-to-public transition

1. Complete every gate in
   [the public-release audit](public-release-audit.md) before changing
   visibility. In particular, licensing, history/privacy, issue review,
   security policy, branch protection, and an explicit publication allow-list
   precede public distribution.
2. Review and, where necessary, delete old workflow runs and artifacts before
   publication. GitHub states that when a private repository becomes public,
   its code, activity, and Actions history/logs become public, anyone can fork
   it, private forks are detached, push rulesets are disabled, and stars and
   watchers are erased
   ([repository visibility](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/managing-repository-settings/setting-repository-visibility)).
3. Re-establish and verify the intended rulesets/branch protection after the
   visibility change because GitHub explicitly disables push rulesets during a
   private-to-public transition.
4. Make the repository public before relying on free GitHub artifact
   attestations. On Free, Pro, and Team plans, attestations are available only
   for public repositories
   ([GitHub security features](https://docs.github.com/en/code-security/getting-started/github-security-features#artifact-attestations)).
5. Publish neither a tap formula nor an installer that names release assets
   until the public repository URL, module path, release owner, license, and
   support policy are final. Those identifiers become upgrade dependencies.

## Recommendations

These improve trust or operability but are not necessary to establish the
initial two-channel contract.

- Reproduce all four archives twice in clean jobs and compare digests before
  publishing. Go build metadata and archive timestamps must be controlled for
  byte-for-byte reproducibility.
- Generate an SBOM attestation as well as build provenance. GitHub's attestation
  action supports SBOM predicates, but end users do not need an SBOM to install
  the first release
  ([artifact attestations](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations#generating-an-attestation-for-an-sbom)).
- Use a GitHub Actions environment for release/signing secrets with required
  reviewer approval, and separate unsigned PR CI from trusted release jobs.
- Run target-native smoke tests in clean macOS Intel, macOS ARM64, Linux x86-64,
  and Linux ARM64 environments. Emulated builds are useful but Go explicitly
  says QEMU user mode is not an officially supported Linux platform
  ([Go minimum requirements](https://go.dev/wiki/MinimumRequirements#operating-systems)).
- Publish an installer checksum or signature and a stable, versioned installer
  URL. This cannot make `curl | sh` intrinsically pre-verified, but it enables
  review, pinning, and offline verification.
- Add `skillet version` (or reliable `--version`) before distribution so the
  tap, installer, support reports, and post-install tests can identify the exact
  build without launching the interactive TUI.
- Once adoption justifies it, evaluate a source-building `homebrew/core`
  formula. Do not make core acceptance a prerequisite for the project-owned
  tap.

## Unknowns to resolve during implementation

1. **Apple credentials:** whether the maintainer already has an Apple Developer
   Program membership and a usable Developer ID Application certificate.
2. **macOS container:** whether notarizing a ZIP containing the command-line
   executable gives the desired Gatekeeper experience on every supported macOS
   version, or whether the initial macOS artifact should be a signed/stapled
   `.pkg` or disk image. This needs a clean-machine quarantine test.
3. **Version command:** the current product's exact noninteractive version
   interface and build-time injection mechanism. The release and Homebrew tests
   need one.
4. **Release trigger:** whether a protected version-tag push creates a draft
   release or a maintainer first creates the draft. Either can satisfy the
   trust boundary; the governance ticket should choose one owner and rollback
   procedure.
5. **Tap automation identity:** whether the release workflow may update the
   separate tap through a narrowly scoped GitHub App, or a maintainer will open
   the formula update. The repository-scoped `GITHUB_TOKEN` cannot write to an
   unrelated tap repository
   ([`GITHUB_TOKEN`](https://docs.github.com/en/actions/concepts/security/github_token)).
6. **Direct-install destination:** the precise default when no user-writable
   PATH directory exists. `$HOME/.local/bin` is the recommended no-root
   fallback, but public docs must state it and its PATH consequences.

## Acceptance evidence for the implementation tickets

The distribution route is not proven merely by a green cross-build. Before it
is advertised, a release candidate must demonstrate:

- all four archives build from one immutable version tag;
- checksums match the uploaded bytes and a deliberately corrupted archive is
  rejected;
- provenance verifies against the exact release workflow;
- both macOS binaries pass signature, notarization, Gatekeeper, and interactive
  launch checks after a quarantined download;
- both Linux binaries launch natively on the advertised architectures and OS
  floor;
- a fresh `brew install`, a later `brew upgrade`, and uninstall work on the
  four Homebrew targets;
- a fresh direct install, same-channel pinned install, later upgrade, collision
  with a Homebrew-managed binary, unsupported-platform failure, and checksum
  failure all behave as documented; and
- a fresh public clone exposes no private workflow artifacts or unintended
  files and has the intended post-transition rulesets active.
