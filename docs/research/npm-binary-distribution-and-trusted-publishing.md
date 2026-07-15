# npm binary distribution and trusted publishing

Researched 2026-07-10 for [Specify npm binary distribution and trusted
publishing](https://github.com/jnyross/Skill-Manager/issues/43).

## Decision

Publish five public, same-version packages:

| Role | Package | npm platform selectors | Payload |
|---|---|---|---|
| Launcher | `@jnyross/skillet` | none | `bin/skillet.cjs`, README, LICENSE |
| macOS Apple Silicon | `@jnyross/skillet-darwin-arm64` | `os: ["darwin"]`, `cpu: ["arm64"]` | `bin/skillet`, README, LICENSE, THIRD_PARTY_NOTICES |
| macOS Intel | `@jnyross/skillet-darwin-x64` | `os: ["darwin"]`, `cpu: ["x64"]` | same |
| Linux ARM64 | `@jnyross/skillet-linux-arm64` | `os: ["linux"]`, `cpu: ["arm64"]` | same |
| Linux x86-64 | `@jnyross/skillet-linux-x64` | `os: ["linux"]`, `cpu: ["x64"]` | same |

`x64` is the npm/Node architecture name (`process.arch`); its Go build input is
`GOARCH=amd64`. The Linux packages deliberately omit npm's `libc` selector
because Skillet's release binary is built with `CGO_ENABLED=0` and the support
contract is not glibc-only.

The launcher declares all four platform packages in `optionalDependencies`,
each pinned to the launcher's exact version. npm uses `os` and `cpu` to select
the installable package, and optional dependencies allow the three nonmatching
packages to be skipped rather than failing the whole install. These fields and
the global `bin` link behavior are defined by npm's
[`package.json` documentation](https://docs.npmjs.com/files/package.json/).

All five proposed names returned registry `E404` on 2026-07-10. That is useful
pre-bootstrap evidence, not a reservation; only the authenticated owner of the
`@jnyross` scope can establish the names.

## Repository and package layout

The implementation should use this fixed source layout:

```text
packaging/npm/
  package.json                 # private workspace root; never published
  packages/
    skillet/
      package.json
      bin/skillet.cjs
      README.md
      LICENSE
    darwin-arm64/
      package.json
      bin/skillet
      README.md
      LICENSE
      THIRD_PARTY_NOTICES
    darwin-x64/                # same shape
    linux-arm64/               # same shape
    linux-x64/                 # same shape
```

Every public `package.json` must contain the same version, `license: "MIT"`,
the exact-case public repository URL
`git+https://github.com/jnyross/Skill-Manager.git`, its workspace `directory`,
and an explicit `files` allow-list. npm recommends `npm pack --dry-run` to
inspect the exact publication set; the release gate must additionally inspect
the produced tarball, not only the dry-run listing
([npm publish and included files](https://docs.npmjs.com/cli/publish/)).

The launcher package has:

```json
{
  "name": "@jnyross/skillet",
  "version": "X.Y.Z",
  "bin": { "skillet": "bin/skillet.cjs" },
  "engines": { "node": ">=22.14.0", "npm": ">=10.9.0" },
  "optionalDependencies": {
    "@jnyross/skillet-darwin-arm64": "X.Y.Z",
    "@jnyross/skillet-darwin-x64": "X.Y.Z",
    "@jnyross/skillet-linux-arm64": "X.Y.Z",
    "@jnyross/skillet-linux-x64": "X.Y.Z"
  }
}
```

The package contains no `preinstall`, `install`, or `postinstall` script. A
normal or `--ignore-scripts` installation must behave identically. This avoids
making arbitrary lifecycle code part of the trust boundary.

## Launcher contract

`bin/skillet.cjs` starts with `#!/usr/bin/env node` as npm requires for a
JavaScript `bin` entry. It must:

1. Map only the four supported `process.platform` / `process.arch` pairs to the
   corresponding package name. Anything else exits nonzero and lists the four
   supported pairs; it never guesses, emulates, downloads, or builds.
2. Resolve the chosen package relative to the launcher with Node module
   resolution, not with a global-prefix assumption.
3. Read the selected package's `package.json` and require its version to equal
   the launcher's version exactly before executing anything.
4. Require `bin/skillet` to exist and be executable, then launch it directly
   without a shell, passing the original arguments and inheriting stdin,
   stdout, stderr, and terminal behavior.
5. Propagate the child's exit status or terminating signal.
6. If the optional package is missing, exit nonzero with the detected pair,
   expected package and version, and this recovery command:
   `npm install --global @jnyross/skillet@<version> --include=optional`.

npm installs optional dependencies by default but permits callers or config to
omit them. `--include=optional` overrides an omit setting
([npm install configuration](https://docs.npmjs.com/cli/install/)). A missing
platform package must therefore be a first-class diagnostic, not an uncaught
`MODULE_NOT_FOUND` exception.

The launcher is intentionally small and has no runtime dependency beyond Node.
The Go executable remains the product; Node is only the distribution shim.

## Runtime and publishing tool baselines

These are two different contracts:

- **Consumer:** Node `>=22.14.0` and npm `>=10.9.0`. Node 20 is already EOL;
  Node 22 remains LTS, and production applications should use supported LTS
  lines ([Node release status](https://nodejs.org/en/about/previous-releases)).
  The chosen floor is exercised locally by Node `v22.22.3` and npm `10.9.8`.
  CI must also test the current Node LTS/npm pair. `engines` declares this
  support boundary, while the release tests enforce it.
- **Publisher:** Node `>=22.14.0` and npm CLI `>=11.5.1`. These are npm's stated
  minimums for trusted publishing, not requirements imposed on consumers
  ([npm trusted publishing](https://docs.npmjs.com/trusted-publishers/)). The
  release workflow should pin one reviewed Node 24 LTS version and one exact npm
  11 version meeting this floor instead of resolving `latest` during release.

Raising either consumer floor is a compatibility decision and needs release
notes. Updating the publisher toolchain alone does not raise the consumer
floor.

## Version and publication invariants

For Git tag `vX.Y.Z` (or `vX.Y.Z-rc.N`):

- the Go binary reports `X.Y.Z` (or the full prerelease);
- all five `package.json` files use that exact version;
- all four launcher `optionalDependencies` use that exact version, never a
  range, tag, workspace protocol, or `latest`;
- GitHub Release tag/title, npm tarball names, checksums, and release notes name
  the same version;
- the workflow fails before publishing if any value differs.

Build each Go executable once from the tagged commit, copy that tested byte
sequence into its platform package, set executable mode, and run `npm pack`
once per package. The exact five resulting `.tgz` files are the files tested,
checksummed, attached to the draft GitHub Release, and passed to `npm publish`;
the publish job must not rebuild or repack them.

Publication order is part of correctness:

1. Build, test, pack, checksum, and create the complete draft GitHub Release.
2. Pass the protected `npm-publish` GitHub environment approval.
3. Publish the four platform package tarballs first.
4. On each native target, globally install the still-local launcher tarball;
   npm then resolves the already-public exact platform package. Run the real
   smoke tests.
5. Publish the immutable GitHub Release and verify its assets.
6. Publish the launcher tarball last. For a stable version, this is the only
   action that advances `latest`; prereleases use a non-`latest` tag such as
   `next`.

npm publishes to `latest` unless another `--tag` is supplied, and an unqualified
install resolves `latest`
([npm dist-tags](https://docs.npmjs.com/adding-dist-tags-to-packages/)). Launcher
last prevents users from resolving a version before its native package exists.
Publishing the GitHub Release before the launcher also ensures the canonical
notes and immutable assets exist before the user-facing install route advances.

Published `name@version` identities are immutable and must never be reused,
even after unpublishing. If a published tarball is wrong, deprecate the affected
version and issue a new version rather than deleting or replacing bytes
([npm unpublish policy](https://docs.npmjs.com/unpublishing-packages-from-the-registry/),
[npm deprecation guidance](https://docs.npmjs.com/deprecating-and-undeprecating-packages-or-package-versions/)).

## Trusted publisher and provenance contract

Each of the five npm packages needs its own trusted-publisher configuration;
npm allows one trusted publisher per package. Configure all five with:

- provider: GitHub Actions;
- owner: `jnyross`;
- repository: `Skill-Manager`;
- workflow filename: `release.yml` (filename only);
- environment: `npm-publish`;
- allowed action: `npm publish`.

The publish job must run on a GitHub-hosted runner, use `permissions:
contents: read` and `id-token: write`, and provide no `NODE_AUTH_TOKEN`, npm
write token, or other registry credential. The separate GitHub Release job may
hold `contents: write`; the npm job must not inherit it. npm validates the
configured owner, repository, workflow filename, environment, and exact-case
`repository.url`. Trusted publishing automatically creates provenance for a
public package published from a public repository; it cannot create that
provenance while this repository remains private
([trusted-publisher requirements and limitations](https://docs.npmjs.com/trusted-publishers/),
[npm provenance](https://docs.npmjs.com/generating-provenance-statements/)).

The `npm-publish` GitHub environment permits only `v*` tags, requires explicit
maintainer approval, and disallows administrator bypass. GitHub environments
can gate a job on reviewers and selected tag patterns
([GitHub deployment environments](https://docs.github.com/en/actions/reference/workflows-and-actions/deployments-and-environments)).

Configure direct `npm publish`, not stage-only publishing, because the already
accepted release governance uses the protected GitHub environment as its one
human approval boundary. Stage-only trusted publishing is a valid future
hardening option, but it would add separate npm 2FA approvals to every release.

## One-time manual npm bootstrap boundary

Trusted publishing cannot create a package: npm's trust command requires the
package to exist first
([`npm trust` prerequisites](https://docs.npmjs.com/cli/v11/commands/npm-trust/)).
The one-time boundary is therefore:

1. The maintainer creates or verifies control of the npm user `jnyross`, enables
   account 2FA/passkeys, and authenticates locally with `npm login`. The current
   machine returned `E401` to `npm whoami` on 2026-07-10, so authentication is
   not currently established here.
2. After the repository publication gates pass, build five minimal
   `0.0.0-bootstrap.0` packages containing only reviewed metadata and a README
   saying they reserve the namespace and are not a Skillet release. Publish
   each interactively with 2FA using `npm publish --access public --tag
   bootstrap`. Scoped public packages require explicit public access on first
   publish ([scoped public package publishing](https://docs.npmjs.com/creating-and-publishing-scoped-public-packages/)).
   The non-`latest` tag ensures the ordinary install command does not select
   this bootstrap version.
3. Configure the trusted publisher above on each package, through npmjs.com or
   authenticated `npm trust github`. No automation token is created or stored.
4. Publish `0.1.0-rc.1` through the real protected OIDC workflow under `next`,
   verify provenance and the full native install matrix, then set every package
   to **Require two-factor authentication and disallow tokens**. npm recommends
   this setting after trusted publishing is verified.
5. Publish `0.1.0` through the same OIDC workflow. Routine releases thereafter
   require only the GitHub environment approval; npm login/2FA returns only for
   trusted-publisher or package-settings changes and recovery.

If `jnyross` cannot control that npm user scope, stop before publication and
change ADR 0006, every package name, and all documentation together. Do not
silently substitute an organization or a different scope during bootstrap.

## GitHub Release relationship

The GitHub Release remains the canonical changelog and immutable release
record. Attach the exact five npm tarballs, `checksums.txt`, and the approved
license/notice material to the draft before publishing it. GitHub recommends
drafting first, attaching every asset, then publishing when immutable releases
are enabled
([GitHub immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases)).

npm remains the supported installation channel; the GitHub assets are evidence
and recovery inputs, not a separately supported direct-install channel. npm's
registry integrity protects fetched tarballs, npm provenance links each package
to the workflow and source, and GitHub checksums/immutability provide an
independent byte ledger for the same packed artifacts.

## Clean-machine verification contract

Every test uses a fresh npm prefix and cache, verifies the expected package
tree, and invokes the installed command rather than a workspace binary.

### Pre-publication deterministic tests

- Unit-test all four platform mappings plus unsupported OS/CPU, missing package,
  version mismatch, missing executable, child exit code, and signal paths.
- Use an ephemeral local npm registry to publish all five candidate packages.
  On native Darwin/Linux arm64/x64 jobs, run the exact global install and
  upgrade commands against it and assert only the matching optional package is
  installed.
- Install with `--omit=optional`; invocation must fail with the specified
  `--include=optional` recovery, not a stack trace.
- Install a known-good version, make the registry/network fail during a newer
  global install, and assert the old `skillet --version` and clean start/quit
  still work. npm documents installation behavior but does not promise this
  rollback property. If any supported npm version fails this empirical gate,
  the release is blocked and ADR 0006 must be revisited; do not weaken the
  earlier failed-upgrade decision in documentation.
- Simulate a partial publish and prove the launcher is never published or
  tagged while one exact platform package is absent.

### Native production-registry release-candidate tests

Run the provenance-backed release candidate on:

- GitHub-hosted macOS Intel and arm64 runners;
- Ubuntu LTS x64 and arm64 runners;
- Debian stable x64 and arm64 containers on matching native Linux runners.

GitHub currently exposes native x64/arm64 Linux and Intel/arm64 macOS hosted
runner labels ([GitHub-hosted runners](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)).
The first public release also needs a clean macOS 12 test on Intel and Apple
Silicon, because hosted images do not prove the selected minimum OS floor.

On each lane:

1. Install the consumer floor (Node 22.14+ with npm 10.9.x), then repeat the
   launcher tests on the current Node LTS/npm pair.
2. Set fresh `NPM_CONFIG_PREFIX` and cache directories and put its bin directory
   on `PATH`.
3. Run `npm install --global @jnyross/skillet@<rc> --include=optional`.
4. Assert exactly one matching platform package, executable mode, package and
   binary version identity, `skillet --version`, and a real start-and-clean-quit
   TUI smoke.
5. For later releases, install the previous stable first, upgrade to the RC,
   and prove the same checks plus preservation of `~/.skillet` data. For
   `0.1.0`, test RC-to-stable because no previous stable exists.
6. Run `npm audit signatures` and verify the registry shows provenance for all
   five packages.

No build-only, tarball-only, or launcher-unit test substitutes for this real
installed command path.

## Partial failure and recovery

- A failure before any publish changes no registry state.
- If only some platform packages publish, do not publish the GitHub Release or
  launcher. Retry only missing packages after verifying already-published
  tarball integrity matches the recorded checksum.
- If all platform packages and the GitHub Release exist but launcher publish
  fails, retry publishing the identical launcher tarball; `latest` has not
  moved.
- If any published package bytes or metadata are wrong, do not overwrite,
  unpublish, or retag the version. Deprecate all five packages at that version
  with one recovery message, leave the GitHub evidence intact, and forward-fix
  with a new SemVer version.
- If a post-publication smoke fails after `latest` moves, publish a new patch.
  Users may explicitly reinstall the last known-good version with
  `npm install --global @jnyross/skillet@<known-good> --include=optional`.

## Downstream implementation gate

The release-pipeline design can treat this ticket as complete only if it
implements and verifies:

- the exact five names, layouts, selectors, allow-lists, and no-script policy;
- launcher mapping, same-version validation, transparent execution, and clear
  missing/unsupported diagnostics;
- one source of version truth and a fail-closed five-package consistency check;
- build-once/pack-once artifacts, platform-first/launcher-last publication;
- consumer and publisher toolchain floors as separate contracts;
- five trusted publishers with exact workflow/environment identity, no npm
  write token, and provenance on every real version;
- the one-time interactive bootstrap and first OIDC release-candidate proof;
- GitHub draft/immutable release ordering and checksums of the exact npm
  tarballs;
- local-registry failure tests plus native registry clean install, upgrade, and
  start/quit tests across the supported architecture matrix;
- release blocking, deprecation, and forward-fix behavior for every partial
  failure point.

## Residual uncertainty

The only deliberately unresolved external behavior is npm's preservation of a
previous global executable when an upgrade is interrupted or fails. No primary
npm documentation found in this research makes that transactional guarantee.
The contract therefore turns it into a required executable test against every
supported npm line. Failure is a distribution-design blocker, not a reason to
silently lower the product guarantee.
