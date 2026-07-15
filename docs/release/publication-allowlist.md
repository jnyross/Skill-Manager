# Public repository allow-list

The publication commit is assembled intentionally. Bulk staging is rejected.

Allowed categories are root project documentation and policies; Go source,
tests, `go.mod`, and `go.sum`; `.github` workflows and dependency configuration;
`docs/agents`, `docs/adr`, `docs/research`, and `docs/release`; npm packaging
source under `packaging/npm` except generated binaries, caches, artifacts, and
tarballs; and repository instructions that contain no machine-private state.

Explicitly excluded are `.agents`, `.claude`, `.codex`, root `SKILL.md`,
`skills-lock.json`, scheduled-task locks, `.ralph`, secrets, environment files,
editor state, generated native binaries, npm tarballs, caches, and private Git
refs. Every staged path must be compared with this allow-list before commit.
