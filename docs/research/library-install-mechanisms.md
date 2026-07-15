# Library install mechanisms (skills.sh + Claude Code plugin install)

Primary-source research for Skillet phase 3 Install.
Researched 2026-07-09 against official docs, package README/CLI help, and live local experiments in isolated temp directories (real `~/.claude/skills`, `~/.codex`, and the Skills Manager repo skill trees were not modified).

Status: complete for both questions. Tool versions at research time: `skills` CLI **1.5.15** (`npx skills --version`); Claude Code **2.1.201** (`claude --version`).

## skills.sh

### Documented interface

Public site docs remain thin on scope:

- https://www.skills.sh/ — install example is `$ npx skills add <owner/repo>`; no project vs global distinction on the homepage (docs-only; fetched 2026-07-09).
- https://www.skills.sh/docs — same basic command (`npx skills add vercel-labs/agent-skills`); notes the CLI is open source at github.com/vercel-labs/skills; no install-scope flags documented on the site page itself (docs-only; fetched 2026-07-09).

The **authoritative install surface** is the `skills` npm package CLI and its README:

- Verified locally: `npx skills --help` / `npx skills add --help` (package version 1.5.15).
- Package README: https://github.com/vercel-labs/skills (also shipped inside the npm tarball as `package/README.md`).

**Exact install command shape** (from CLI help + README):

```text
npx skills add <source> [options]
# aliases: skills a
```

**Source formats** (README, verified locally via CLI help examples):

| Form | Example |
|---|---|
| GitHub shorthand | `vercel-labs/agent-skills` |
| Full GitHub URL | `https://github.com/vercel-labs/agent-skills` |
| Path inside a GitHub tree | `https://github.com/.../tree/main/skills/web-design-guidelines` |
| GitLab / any git URL | `https://gitlab.com/org/repo`, `git@github.com:owner/repo.git` |
| Local path | `./my-local-skills` |

**Add options** (CLI help, verified locally):

| Flag | Meaning |
|---|---|
| `-g, --global` | Install skill **globally (user-level)** instead of project-level |
| `-a, --agent <agents>` | Target agent(s); `*` for all |
| `-s, --skill <skills>` | Install named skill(s); `*` for all |
| `-l, --list` | List skills in the source without installing |
| `-y, --yes` | Skip confirmation prompts |
| `--copy` | Copy files instead of symlinking to agent directories |
| `--all` | Shorthand for `--skill '*' --agent '*' -y` |
| `--full-depth` | Search all subdirectories even when a root SKILL.md exists |
| `--subagent <names>` | Eve subagents only |

**Scope table** (package README "Installation Scope", docs-only relative to site but primary for the CLI):

| Scope | Flag | Location (README wording) |
|---|---|---|
| Project (default) | _(none)_ | `./<agent>/skills/` |
| Global | `-g` | `~/<agent>/skills/` |

**Non-interactive / CI example** from README (docs-only in README, matches CLI flags):

```bash
npx skills add vercel-labs/agent-skills --skill frontend-design -g -a claude-code -y
```

Related commands that matter for observability/updates: `skills list` (`-g` for global), `skills update` (`-g`/`-p`/`-y`), `skills experimental_install` (restore from `skills-lock.json`), `skills remove`.

### Live install experiment

All runs used isolated temp dirs; real home skill trees were checked afterward and remained clean of the probe skill `find-skills`.

#### A. Project install (cwd = scratch)

Command (verified locally):

```bash
cd "$(mktemp -d /tmp/skillet-skills-probe-XXXXXX)"
npx skills add vercel-labs/skills --skill find-skills --agent claude-code -y
```

Observed:

1. CLI resolved source as **`https://github.com/vercel-labs/skills.git`** (printed "Source: …"), not an opaque skills.sh package blob.
2. Security assessment table printed (Gen/Socket/Snyk) with a link back to `https://skills.sh/vercel-labs/skills` (leaderboard/details page).
3. Exit code **0**.
4. Files created under the scratch cwd only:
   - `./.claude/skills/find-skills/SKILL.md` (regular file copy, 5519 bytes)
   - `./skills-lock.json` (project lockfile)
5. No `~/.claude/skills/find-skills` on the real home tree.
6. Summary UI said canonical path `./.agents/skills/find-skills` then `copy → Claude Code`; final tree retained only the Claude path + lockfile (no lingering `.agents/skills/` tree for this single-agent run).

`skills-lock.json` contents (verbatim, verified locally):

```json
{
  "version": 1,
  "skills": {
    "find-skills": {
      "source": "vercel-labs/skills",
      "sourceType": "github",
      "skillPath": "skills/find-skills/SKILL.md",
      "computedHash": "781bd6d3f9b19f8c9af6b53d8d0e4876d0183841b565db34ca7092ffa412d111"
    }
  }
}
```

(Hash field present; no git commit SHA field in the project lock. Exact hash algorithm is content of the installed skill folder via `computeSkillFolderHash` in the CLI bundle — verified by string/inspection of `skills@1.5.15` `dist/cli.mjs`, not by re-deriving the hash.)

Note: the hash string above is from the live file; if you re-run, content-addressed values may change only if upstream skill content changes.

#### B. Global install (isolated `HOME`)

Command (verified locally; `HOME`/`XDG_*` pointed at empty temps so real home was not written):

```bash
export HOME="$(mktemp -d ...)"
cd "$(mktemp -d ...)"   # empty project cwd
npx skills add vercel-labs/skills --skill find-skills --agent claude-code -g -y
```

Observed:

1. Summary: `~/.agents/skills/find-skills` then `copy → Claude Code`.
2. Final install path reported: `~/.claude/skills/find-skills` (under the isolated HOME).
3. Files under isolated HOME:
   - `$HOME/.claude/skills/find-skills/SKILL.md`
   - `$HOME/.agents/.skill-lock.json` (global lock; different schema than project lock)
4. Project cwd stayed empty of skill files (global install is not project-relative).
5. Real `$HOME/.claude/skills/find-skills` still absent.

Global lock (verbatim, verified locally):

```json
{
  "version": 3,
  "skills": {
    "find-skills": {
      "source": "vercel-labs/skills",
      "sourceType": "github",
      "sourceUrl": "https://github.com/vercel-labs/skills.git",
      "skillPath": "skills/find-skills/SKILL.md",
      "skillFolderHash": "eb6a23305aea6e340d14b9de3766e721f9f4861b",
      "installedAt": "2026-07-08T23:14:14.441Z",
      "updatedAt": "2026-07-08T23:14:14.441Z"
    }
  },
  "dismissed": {}
}
```

Global lock path resolution (from `skills@1.5.15` `dist/cli.mjs`, verified by source inspection):

- If `XDG_STATE_HOME` is set: `$XDG_STATE_HOME/skills/.skill-lock.json`
- Else: `$HOME/.agents/.skill-lock.json`

#### C. Multi-agent layout quirk

With `--agent claude-code --agent codex -y` (project, verified locally):

- Summary claimed: `./.agents/skills/find-skills` as `universal: Codex` and `symlink → Claude Code`.
- On-disk after install: only `./.agents/skills/find-skills/SKILL.md` + `skills-lock.json`.
- **No** `./.claude/skills/...` path and **no** symlink found via `find -type l`.

**Consequence:** multi-agent installs are not a safe default for Skillet if the goal is a Claude Code personal/project skill tree. Prefer a single `--agent` matching the Library entry's tool.

#### D. `--copy` single-agent (project)

`npx skills add vercel-labs/skills --skill find-skills --agent claude-code --copy -y` produced the same clean layout as (A): `./.claude/skills/find-skills/SKILL.md` + `skills-lock.json` (verified locally).

### Targeting model (cwd / flags / Personal implications)

| Question | Answer | Evidence class |
|---|---|---|
| Does project install go relative to **cwd**? | **Yes.** Project (default) writes under the process cwd's agent skill dirs (for Claude Code: `./.claude/skills/<name>/`). | verified locally (A) |
| Is there a global/user-level flag? | **Yes: `-g` / `--global`.** | CLI help + README + verified locally (B) |
| Is every install project-relative? | **No.** Global installs go to user-level agent skill dirs (`~/.claude/skills` for Claude Code). | verified locally (B) |
| How should Personal-target Install work? | Shell out with **`-g`** (and pin agent). No need for "cwd trick into `~/.claude`" or install-to-scratch-then-copy, unless Skillet wants to avoid the skills CLI's lockfile side effects in home. | verified locally + CLI help |
| How should Project-target Install work? | Shell out **without** `-g`, with `cwd` = the chosen repo root (so `./.claude/skills` lands in that repo). | verified locally (A) |
| Claude Code global path respects `CLAUDE_CONFIG_DIR`? | CLI source sets `claudeHome = process.env.CLAUDE_CONFIG_DIR?.trim() \|\| join(home, ".claude")` then `globalSkillsDir: join(claudeHome, "skills")`. | inferred from package source inspection of `skills@1.5.15` `dist/cli.mjs` (not live-tested with `CLAUDE_CONFIG_DIR` for skills) |

**Canonical vs agent dirs (source inspection + live):**

- Canonical project skills dir is `.agents/skills` (`AGENTS_DIR$1 = ".agents"`).
- Claude Code agent project dir is `.claude/skills`; global is `$claudeHome/skills`.
- Default `installMode` in code is `"symlink"` (`options.mode ?? "symlink"`); `--copy` forces copies.
- Live single-agent Claude installs with `-y` **copied** into `.claude/skills` and did not leave a separate `.agents/skills` tree. Multi-agent behavior differs (see C). Prefer explicit `--agent` and, for determinism, `--copy`.

### Version / pin observability

| Artifact | When | Pin content |
|---|---|---|
| `skills-lock.json` (cwd) | Project installs | `source`, `sourceType`, `skillPath`, `computedHash` (content hash of skill folder). **No git commit SHA.** version key `1` in live run. |
| `~/.agents/.skill-lock.json` (or XDG state path) | Global installs | `source`, `sourceType`, `sourceUrl`, `skillPath`, `skillFolderHash`, `installedAt`, `updatedAt`. **No git commit SHA.** version key `3` in live run. |
| Installed `SKILL.md` tree | Always | Ordinary files; no embedded skills.sh version metadata observed beyond skill frontmatter itself. |

**What `owner/repo` resolves to:**

- **Direct GitHub clone**, not a proprietary skills.sh content registry.
- Live output: `Source: https://github.com/vercel-labs/skills.git`.
- Source parser in CLI builds `https://github.com/${owner}/${repo}.git` for shorthand (package source inspection).
- skills.sh website is a **discovery/leaderboard + telemetry** front (install counts); security assessment details also deep-link to `https://skills.sh/<owner>/<repo>`.
- CLI also accepts full git URLs and local paths (README), so "skills.sh source" in Skillet is really "skills CLI source string", usually `owner/repo`.

### Gaps

1. **Multi-skill repos:** many leaderboard entries are multi-skill packages. CLI requires `-s/--skill` (or `*` / interactive pick). SPEC's `LibrarySource` currently has only `SkillsShRepo` (`owner/repo`) and no skill-name field — Part 3 cannot install a single skill from a multi-skill repo without extending the data model or always installing `*`.
2. **Default agent selection without `-a`:** not live-tested non-interactively; README warns about multi-agent installs. Skillet should always pass `-a`.
3. **Symlink vs copy selection under `-y`:** code default is symlink; live single-agent runs showed copy. Exact decision tree not fully reverse-engineered. Mitigation: pass `--copy` explicitly for predictable independent trees that match Skillet's "installed copy walks away from Library" model (ADR 0004).
4. **Project lock vs global lock schema drift** (v1 `computedHash` vs v3 `skillFolderHash` + timestamps): treat as CLI-owned; Skillet should not depend on lockfiles for its own Library version display (ADR 0004 already rejects tracking).
5. **Private GitHub / SSO failures:** CLI has dedicated error strings for SAML SSO and auth (source inspection); not live-tested.
6. **Whether `skills update` mutates hashes without commit pins:** not live-tested; inferred that update re-clones latest.
7. **Codex global path via skills CLI:** maps to `$CODEX_HOME/skills` or similar via `codexHome` in agent table (source inspection); not live-tested in this spike. For Claude-only Library skills, irrelevant.
8. **Telemetry:** site docs say anonymous install telemetry; no opt-out flag found in `--help`. Gap for privacy-sensitive automation (docs-only + help).

## Claude Code plugin install

### Documented interface

Primary docs:

- https://code.claude.com/docs/en/plugins-reference.md — CLI commands reference; installation scopes table (`user` → `~/.claude/settings.json`, `project` → `.claude/settings.json`, `local` → `.claude/settings.local.json`, `managed` → managed settings).
- https://code.claude.com/docs/en/discover-plugins.md — two-step model: **add marketplace**, then **install plugin**; non-interactive note: use `claude plugin install` shell command (defaults to user scope unless `--scope` is passed).

Documented install identifier form: `plugin-name@marketplace-name` (discover-plugins; plugins-reference).

Documented marketplace add sources (discover-plugins): GitHub `owner/repo`, git URLs, local paths, remote `marketplace.json` URLs.

### CLI help (verbatim summary)

Verified locally against Claude Code 2.1.201:

```text
claude plugin install|i [options] <plugin>
  Install a plugin from available marketplaces
  (use plugin@marketplace for specific marketplace)

  --config <key=value>   Set a userConfig option from the plugin manifest
  -s, --scope <scope>    user | project | local   (default: "user")
```

```text
claude plugin marketplace add [options] <source>
  Add a marketplace from a URL, path, or GitHub repo

  --scope <scope>        user (default) | project | local
  --sparse <paths...>    sparse-checkout paths for monorepos
```

Also present and relevant: `plugin list [--json] [--available]`, `plugin enable|disable|uninstall|update`, `plugin marketplace list|update|remove`.

`claude plugin install` **does exist** and is the scriptable path referenced in docs.

### Live / dry-run experiment

Isolation method (verified locally): empty temp `HOME` + `CLAUDE_CONFIG_DIR` pointing at a temp config root. Real `~/.claude` was not used as the write target for these probes. `CLAUDE_CONFIG_DIR` is recognized by the binary (string present in CLI; live writes went under the temp config dir).

#### 1. Install with no marketplaces configured

```bash
claude plugin install commit-commands@claude-code-plugins --scope user
```

- Result: **failure** message that the plugin was not found in marketplace `claude-code-plugins` (suggests `marketplace update`).
- Exit code: **1** (verified locally).
- `claude plugin marketplace list` → `No marketplaces configured`.
- `claude plugin list --json` → `[]`.

**Install does not add a marketplace inline.** Marketplace must already be known.

#### 2. Add marketplace, then install (user scope)

```bash
claude plugin marketplace add anthropics/claude-code
# → Successfully added marketplace: claude-code-plugins (declared in user settings)
claude plugin install commit-commands@claude-code-plugins --scope user
# → Successfully installed plugin: commit-commands@claude-code-plugins (scope: user)
```

Both steps exit **0**, no TTY prompts (verified locally under isolated config).

On-disk under `$CLAUDE_CONFIG_DIR` (verified locally):

| Path | Role |
|---|---|
| `settings.json` | `extraKnownMarketplaces` + `enabledPlugins` |
| `plugins/known_marketplaces.json` | marketplace name → source + `installLocation` + `lastUpdated` |
| `plugins/marketplaces/claude-code-plugins/` | cloned marketplace catalog |
| `plugins/installed_plugins.json` | plugin id → array of install records |
| `plugins/cache/claude-code-plugins/commit-commands/1.0.0/` | cached plugin copy |

`settings.json` after user install (verbatim shape, verified locally):

```json
{
  "extraKnownMarketplaces": {
    "claude-code-plugins": {
      "source": { "source": "github", "repo": "anthropics/claude-code" }
    }
  },
  "enabledPlugins": {
    "commit-commands@claude-code-plugins": true
  }
}
```

`installed_plugins.json` record (verified locally):

```json
{
  "version": 2,
  "plugins": {
    "commit-commands@claude-code-plugins": [
      {
        "scope": "user",
        "installPath": ".../plugins/cache/claude-code-plugins/commit-commands/1.0.0",
        "version": "1.0.0",
        "installedAt": "2026-07-08T23:14:20.207Z",
        "lastUpdated": "2026-07-08T23:14:20.207Z",
        "gitCommitSha": "be02c39841a59e2ac1f35ac12285def02acdbb5a"
      }
    ]
  }
}
```

This matches the on-disk shape already documented in `docs/research/skill-mechanisms.md` ("Where things live on disk"), now confirmed for the *install* path as well as post-install inventory.

#### 3. Project scope with no pre-existing `.claude/`

Empty project directory (verified locally):

```bash
cd "$(mktemp -d ...)"   # no .claude
claude plugin install commit-commands@claude-code-plugins --scope project
# → success (marketplace already present in user config from step 2)
```

Creates:

```text
./.claude/settings.json
{
  "enabledPlugins": {
    "commit-commands@claude-code-plugins": true
  }
}
```

**CLI creates `.claude/settings.json` if missing.** No pre-seed required. Plugin bytes still live in the user-level plugin cache under `CLAUDE_CONFIG_DIR` / `~/.claude/plugins/cache`; project scope primarily records enablement in project settings (consistent with skill-mechanisms.md + discover-plugins).

#### 4. Marketplace add with `--scope project`

```bash
claude plugin marketplace add anthropics/claude-code --scope project
```

Writes project `.claude/settings.json` with `extraKnownMarketplaces` only (verified locally). Non-interactive, exit 0.

#### 5. Attempted "inline" install via `plugin@owner/repo`

```bash
claude plugin install foo@anthropics/claude-code
```

Failed with "not found in marketplace `anthropics/claude-code`" — treated `anthropics/claude-code` as a **marketplace name**, not a GitHub auto-add. **Does not** auto-register a marketplace from that string.

**Exit-code quirk (gap):** this particular failure printed an error but the probe recorded exit code **0** once; ordinary missing-marketplace / missing-plugin cases returned **1**. Skillet should treat stdout/stderr success markers (`✔ Successfully installed`) as authoritative if exit codes prove inconsistent, or re-verify exit codes per Claude Code version in CI.

### Interaction with marketplaces and settings.json

Two-step model is mandatory for a *new* marketplace:

1. `claude plugin marketplace add <source> [--scope user|project|local]`
   - Registers catalog in `known_marketplaces.json` + declares in the scope's `settings.json` (`extraKnownMarketplaces`).
2. `claude plugin install <plugin>@<marketplace> --scope user|project|local`
   - Copies plugin into cache, appends `installed_plugins.json`, sets `enabledPlugins[<id>]=true` in the target scope's settings file.

If marketplace is already known (user or project `extraKnownMarketplaces` / `known_marketplaces.json`), step 1 can be skipped.

Official Anthropic marketplace auto-availability (discover-plugins) applies to normal user installs of Claude Code; **empty `CLAUDE_CONFIG_DIR` had zero marketplaces**, so automation that assumes a blank config must add marketplaces explicitly.

`enabledPlugins` is written **true** on successful install (live). Plugins that ship `defaultEnabled: false` are documented to install disabled (plugins-reference ≥ 2.1.154) — not live-tested here; Skillet should not assume every install ends enabled without checking.

### Shell-out vs direct file write

| Approach | Pros | Cons |
|---|---|---|
| **Shell out** to `claude plugin marketplace add` + `claude plugin install` | Keeps `known_marketplaces.json`, marketplace clone, cache layout, `installed_plugins.json`, and `enabledPlugins` consistent; matches documented automation path; non-interactive flags exist; creates missing settings files | Depends on `claude` binary on PATH; network for marketplace clone; occasional exit-code ambiguity; may enable plugin always |
| **Direct file write** to the three JSON files + manual cache populate | No binary dependency | Must reimplement clone/cache/version/`gitCommitSha`/orphan cleanup; high drift risk vs Claude Code updates; skill-mechanisms.md already treats cache layout as tool-owned |

**Recommendation: shell-out is preferable.** Live probes show it is scriptable without a TTY when marketplace source and plugin id are known. Direct writes should remain emergency/repair territory only, same spirit as not hand-editing plugin cache for uninstall.

### Gaps

1. **No single command** that both adds an unknown marketplace and installs a plugin from a raw GitHub URL. Library marketplace entries need either a pre-known marketplace name or a two-step Install.
2. **SPEC `LibrarySource` fields** (`Marketplace`, `PluginName`) match `plugin@marketplace` well, but do not encode the marketplace *source* (`github:owner/repo`, URL, path) needed when the marketplace is not already registered. May need an optional marketplace-source field or a precondition that Install fails with a clear error if marketplace is unknown.
3. **`userConfig` prompts:** install supports `--config key=value` for manifest-declared options; plugins that require interactive configure without defaults were not tested. Risk for non-interactive Install of config-heavy plugins.
4. **Exit code inconsistency** on at least one failure path (see experiment §5).
5. **Official marketplace bootstrap** on a brand-new real user home (without `CLAUDE_CONFIG_DIR` override) not re-tested here; docs say official marketplace is auto-available in normal Claude Code usage.
6. **Local scope** (`--scope local` → `.claude/settings.local.json`) not live-tested; flags accept it.
7. **Whether project-scope install requires marketplace declared at project scope** or accepts user-level known marketplaces: live project install succeeded with marketplace only in the isolated user config — **user-level known marketplace is enough** for project-scope plugin install (verified locally). Team-share still wants `extraKnownMarketplaces` in project settings so clones work for others (docs).
8. **Codex plugin install** is out of scope for this spike (question B is Claude Code only).

## Consequences for Skillet Install

Per ADR 0004: Library entries store install-source pointers; Install re-resolves fresh, places a disconnected copy, no ongoing version tracking. Part 0 findings map onto Part 3 dispatch as follows.

### By `LibrarySourceKind`

| Kind | Preferred mechanism | Personal target | Project target | Shell-out? |
|---|---|---|---|---|
| `skills.sh` | `npx skills add <SkillsShRepo> -a <agent> -y [--skill …] [--copy] [-g]` | **`-g`** (writes `~/.claude/skills` or agent global equivalent) | **no `-g`**, `cwd` = `target.RepoRoot` | **Yes** — CLI owns multi-agent layout, lockfiles, fetch |
| `git` | Existing SPEC plan: shallow `git clone` to scratch + copy subpath | Copy into personal skills dir for tool | Copy into project skills dir for tool | Optional (git binary); no skills CLI required |
| `marketplace` | `claude plugin marketplace add` *if needed*, then `claude plugin install <PluginName>@<Marketplace> --scope user\|project` | `--scope user` | `--scope project` (creates `.claude/settings.json` if absent) | **Yes** — strongly preferred over hand-writing JSON/cache |
| `local-path` | Directory copy | Personal skills dir | Project skills dir | No |

### skills.sh — concrete Skillet recipe

```bash
# Personal / Claude Code
npx skills add "$owner_repo" -g -a claude-code -y --copy ${skill:+--skill "$skill"}

# Project / Claude Code (cwd must be repo root)
npx skills add "$owner_repo" -a claude-code -y --copy ${skill:+--skill "$skill"}
```

- Do **not** implement the "cwd = `~/.claude`" guess from the pre-research SPEC wording; **`-g` is the supported Personal mechanism**.
- After install, the placed tree under `.claude/skills/<name>` (or `~/.claude/skills/<name>`) is an ordinary skill; subsequent Manual-only can use existing `SetManualOnly`.
- Ignore or tolerate `skills-lock.json` / `.skill-lock.json` side effects; they are CLI bookkeeping, not Skillet Library state (ADR 0004).
- If Skillet must avoid writing the global lockfile or multi-agent side effects: install with project mode into a scratch cwd + `--copy -a claude-code`, then move `scratch/.claude/skills/<name>` into the real target. Live evidence says `-g` is simpler and correct for Personal.

### Marketplace / plugin — concrete Skillet recipe

```bash
# Ensure marketplace (idempotent enough: re-add may no-op / "already on disk")
claude plugin marketplace add "$marketplace_source" --scope user   # or project for team catalogs

# Install + enable at target scope
claude plugin install "${plugin}@${marketplace}" --scope user      # Personal
claude plugin install "${plugin}@${marketplace}" --scope project   # Project, cwd = repo root
```

- Requires `claude` on PATH.
- Prefer parsing CLI success text and re-reading `installed_plugins.json` / scope `settings.json` to confirm, rather than trusting exit code alone until the exit-code quirk is closed.
- Project install without `.claude/` is fine — CLI creates settings.
- Installing a plugin is **not** the same as installing a free-floating skill; inventory remains via plugin cache + `enabledPlugins` as in skill-mechanisms.md.

### Divergences vs SPEC.md Part 0/3 sketch (do not silently code around)

1. **Personal skills.sh does not require a cwd trick** — use `-g`. SPEC Part 0 text that only contemplates project-relative installs is outdated; Part 3 should dispatch on `-g` vs cwd=repo.
2. **`LibrarySourceSkillsSh` may need a skill name field** (or documented `*` behavior) for multi-skill repos; `owner/repo` alone is under-specified for packages like `vercel-labs/agent-skills` / `mattpocock/skills`.
3. **`LibrarySourceMarketplace` may need marketplace *source* (git/GitHub), not only marketplace name**, so Install can run `marketplace add` when `known_marketplaces.json` lacks the catalog. Name alone is enough only when the marketplace is already registered.
4. **Plugin Install is two CLI calls when marketplace is unknown**, not a single `claude plugin install`.
5. **Always pin `-a` for skills CLI**; unrestricted agent install can land skills outside `.claude/skills` (multi-agent live run).
6. **Prefer `--copy`** for skills.sh so Skillet's overwrite/Manual-only paths operate on real directories, not symlinks into `.agents/skills`.

### What Part 3 can implement without further research

- skills.sh Personal/Project shell-out with the flags above (network + `npx` dependency).
- Plugin install shell-out for already-known marketplaces with `--scope user|project`.
- Local-path and git paths unchanged from SPEC.

### What still needs a product decision (not resolved here)

- Whether Library skills.sh entries store optional skill name(s).
- Whether Library marketplace entries store marketplace source for auto-`marketplace add`.
- Whether Project plugin Install should also write `extraKnownMarketplaces` into the project's settings for teammates (docs recommend it for team marketplaces; live install did not require it for the installing user).
- How aggressively to surface skills CLI security assessment output in the TUI.
