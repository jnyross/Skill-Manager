# Public-release audit

Audited 2026-07-10 and updated against the publication candidate on 2026-07-15
for [Audit the repository for safe public release](https://github.com/jnyross/Skill-Manager/issues/32).

## Verdict

Do not change the repository from private to public yet. The publication
candidate now has an MIT license, public policies, CI and release workflows,
neutral tracked examples, an explicit publication allow-list, and the public Go
module path. No credential was found by the pattern-based scans described
below. Publication still requires a reviewed history rewrite, a fresh-clone
audit, review of public issue content, repository protection/security settings,
and explicit maintainer approval at the visibility boundary.

## Required before publication

1. Decide whether the existing private Git author email may be
   public. It is embedded in every reachable commit inspected. If not, rewrite
   history before the first public push; changing Git configuration only fixes
   future commits.
2. Rewrite or deliberately exclude historical revisions that contain personal
   home/project paths, then repeat the path, email, and secret scans against the
   exact rewritten publication history. The current publication candidate uses
   neutral tracked examples; older reachable revisions still contain the
   original material.
3. Review every existing GitHub issue and comment as public content. The
   automated pattern pass flagged only issue 11 for generic words such as
   `token`; its inspected content contained no credential, but automated scans
   cannot establish author consent or identify every contextual disclosure.
4. Protect `main` and require the release-quality checks selected by this
   Wayfinder map before merging. The default branch is currently unprotected.
5. Enable GitHub dependency vulnerability alerts and automated security fixes,
   or record an explicit reason not to. Both are currently disabled.
6. Assemble and inspect the publication commit from
   `docs/release/publication-allowlist.md`, excluding local agent skills,
   machine state, generated native binaries, tarballs, and private refs. No
   bulk `git add .` is safe for the publication change.
7. Create the pre-migration backup and verify a fresh clone of the exact
   sanitized history before changing remote history or repository visibility.

## Accepted current facts

- The repository has no Actions secrets, repository variables, deploy keys, or
  webhooks listed by the GitHub APIs used in this audit.
- The publication candidate contains no `.env` file, private key, or detected
  provider/GitHub token. `.gitignore` excludes `.env`, `.env.*`, `.DS_Store`,
  built binaries, release artifacts, local agent configuration, and machine
  state.
- `LICENSE`, `SECURITY.md`, `SUPPORT.md`, `CONTRIBUTING.md`, and
  `CODE_OF_CONDUCT.md` are present; distributed Go dependency notices are
  generated and checked from `go list -m -json all`.
- The Go module path is `github.com/jnyross/Skill-Manager`; CI, Dependabot, and
  the protected OIDC/provenance release workflow are present.
- New commits use the maintainer's GitHub noreply identity. The old personal
  email remains only in history and must be handled by the approved history
  migration.
- Stash-like `cmux last turn baseline` objects are present in local `--all`
  history, while `origin/main` is behind local `main`. Ordinary pushes do not
  publish local stash refs, but the exact refs and intended commits must be
  reviewed before the repository is made public.
- No GitHub Release or npm package has been published by this implementation.
  The external publication gates remain issues #61 through #63.

## Evidence collected

- Enumerated tracked and untracked files and inspected recent/all reachable Git
  metadata.
- Searched the current workspace and every reachable revision for common secret
  formats, private-key headers, credential assignments, personal email domains,
  and absolute user paths.
- Inspected repository visibility, license detection, default-branch protection,
  vulnerability-alert state, automated-security-fix state, Actions secret names,
  repository variables, deploy keys, and webhooks through GitHub APIs.
- Inspected the only issue selected by the issue/comment sensitivity pattern
  scan.

Pattern scans reduce risk but do not prove that arbitrary prose or an
unrecognised credential format is safe. The publication change therefore needs
a final human-readable diff and issue review, not only a green scanner.
