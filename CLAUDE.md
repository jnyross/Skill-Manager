# Skills Manager

## Agent skills

### Issue tracker

Issues are tracked in this repo's GitHub Issues via the `gh` CLI; external PRs are not a triage surface. See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage labels are used verbatim (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `CONTEXT.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.

### Default local install and verification

A public `@next` release (`v0.1.0-rc.1`) of `@jnyross/skillet` is now available
on npm. For development and testing, install Skillet from the current working
tree using a fresh isolated Go build cache. The default acceptance path is: run
`go test ./...`, run `go vet ./...`, build `./cmd/skillet` with `CGO_ENABLED=0`
to a temporary candidate, then install that exact tested file to
`~/go/bin/skillet`. Verify the installed file is byte-identical to the candidate
and launch the installed `skillet` command for a real start-and-clean-quit smoke
test. Preserve `~/.skillet`, managed skills, archives, and unrelated working-tree
changes; a local reinstall replaces only the executable. Do not describe this
source workflow as the stable public install channel: ADR 0006 reserves that
contract for `npm install --global @jnyross/skillet@latest`, which remains gated
until the stable release.

### Codex skill discovery

For Skillet's Codex inventory logic, treat `docs/research/skill-mechanisms.md` as the evidence trail. Issue #13 proved with live `codex exec` that ordinary non-`.system` user skills under `$CODEX_HOME/skills` (`~/.codex/skills`) are still discovered despite the public docs omitting that root; do not remove `CodexHome/skills` from scans without a newer live-runtime probe.

### Project skill discovery

For Skillet's Project inventory, keep Claude Code and Codex project-skill roots distinct even though both now walk every ancestor from the working directory through the repository root: Claude uses `.claude/skills`, while Codex 0.144.1 uses `.agents/skills`. Outside a repository, Skillet keeps Codex's bounded cwd/parent fallback and Claude's cwd-only fallback. Treat `docs/research/skill-mechanisms.md` as the evidence trail before changing this split.
