# Release runbook

## Prepare

1. Start from protected `main` with a clean, reviewed tree and passing CI.
2. Choose one unused SemVer. Set all five package manifests and launcher
   optional dependencies to that exact version.
3. Generate third-party notices, build each native executable once, pack each
   package once, inspect `artifacts.json`, and verify `SHA256SUMS`.
4. Run the local-registry gate on native macOS and Linux runners at the consumer
   floor and current LTS toolchains.
5. Create an annotated `vX.Y.Z` tag at the current protected `main` head.

## Approve and publish

The workflow first creates a complete draft GitHub Release. The protected
`npm-publish` environment is the human approval boundary. It publishes the four
native packages, proves the local launcher against those immutable packages,
publishes the GitHub Release, and publishes the launcher last. Stable versions
advance `latest`; prereleases use `next`.

The environment must allow only `v*` tags, require the maintainer, and disallow
administrator bypass. Each npm package must trust only owner `jnyross`, repo
`Skill-Manager`, workflow `release.yml`, environment `npm-publish`, action
`npm publish`. No long-lived npm token is used.

## Observe

Check all five npm versions and provenance, the GitHub Release tag/title/assets,
SHA-256 checksums, native job logs, then install `@jnyross/skillet@<version>` in
a fresh global prefix and run `skillet --version` plus a start/quit smoke.

## Recover

Never replace or reuse published `name@version` bytes. If a native publication
fails, do not publish the launcher. If post-publication verification fails,
leave the prior launcher tag in place, deprecate the bad version where useful,
fix forward under a new SemVer, and record the incident in release notes. A
failed local npm upgrade must leave the prior installed command callable.
