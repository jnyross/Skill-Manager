# Tool Adapters share managed workspace state

Guided setup configures Claude Code and Codex together, but duplicating every
compatible skill and instruction file would create two sources of truth while
blindly sharing every format would hide real Tool differences. Keep the
existing **Tool** term and make each **Tool Adapter** declare its capabilities;
use one managed `AGENTS.md` instruction source and one canonical skill tree for
entries verified compatible with both Tools, with a Claude compatibility link
or thin import and adapter-specific variants only where the mechanisms differ.

A committed, project-relative portion of the **Workspace receipt** records
Managed files, content hashes, Built-in catalog identity, and portable setup
outcome so ownership travels with the repository. Machine-specific executable
and live-verification details remain ignored local state. An unmanaged path may
be adopted only when it is an exact compatible match; otherwise replacement
requires explicit confirmation and a recoverable backup. Required content is
never silently skipped.

Setup reports one of four outcomes: **Blocked** leaves no Managed-file change;
**Configured-unverified** means configuration exists but a missing Tool
executable prevented live proof; **Verified** means both Tool Adapters proved
discovery; and **Partial** is reserved for an external Tool side effect that
remains after reversible Managed-file writes are rolled back and must include a
repair action.

## Considered Options

- **Introduce Harness beside Tool** — rejected because both names would mean
  Claude Code or Codex and weaken the existing Source/Tool model.
- **Maintain separate native copies for every Tool** — rejected for verified
  cross-compatible content because repeat setup could let the copies drift.
- **Keep the entire Workspace receipt local** — rejected because a fresh clone
  could not distinguish user work from paths Skillet is allowed to manage.
- **Continue after required unmanaged conflicts** — rejected because an
  apparently successful workspace could be missing required Bundle members.
