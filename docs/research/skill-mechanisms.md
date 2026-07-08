# Skill mechanisms in Claude Code and Codex

Primary-source research for Skillet's core operations (inventory, uninstall, manual-only, suppress).
Researched 2026-07-08 against official docs and the live installs on John's machine.

Status: complete (both sections); Codex section re-verified 2026-07-08 against a fresh public install (see "Re-verification against codex-cli 0.143.0" below) and a live authenticated Codex CLI runtime probe for issue #13 — the caveat that the original research may reflect a canary build is now discharged.

## Claude Code

### Making a skill manual-only

Two SKILL.md frontmatter fields control invocation:

- `disable-model-invocation: true` — only the user can invoke via `/skill-name`; the model cannot auto-invoke. This is exactly Skillet's "manual-only".
- `user-invocable: false` — the inverse: only the model can invoke; hidden from the `/` menu.

Source: https://code.claude.com/docs/en/skills.md ("Control who invokes a skill", "Frontmatter reference").
Verified locally: `~/.claude/plugins/cache/mattpocock-skills-marketplace/mattpocock-skills/1445797da5ee/skills/in-progress/loop-me/SKILL.md` uses `disable-model-invocation: true`.

There is **no settings.json per-skill disable**. The only skill-related settings are `disableBundledSkills` (kills all built-in skills) and `disableSkillShellExecution`. Verified: `~/.claude/settings.json` contains no per-skill entries.
Source: https://code.claude.com/docs/en/settings.md ("Available settings").

**Consequence for Skillet:** manual-only on personal/project skills = editing the skill's own frontmatter. There is no non-invasive alternative.

### Plugin skills: no supported per-skill control

For skills bundled in plugins there is **no supported way to disable one skill** that survives plugin updates:

- Settings can only enable/disable the entire plugin (`enabledPlugins` map).
- Editing the cached SKILL.md works but is abandoned on update: each plugin version is a separate cache directory; the old one is orphaned and auto-deleted after 7 days.

Source: https://code.claude.com/docs/en/plugins-reference.md ("Plugin caching and file resolution").

**Consequence for Skillet:** Suppress must be a Skillet-owned mechanism. Skillet must record suppression state in its own config and re-apply frontmatter edits when it detects a new plugin version directory (self-healing on each run), or find an equivalent. This is the highest-risk piece of the design.

### Where things live on disk

- Personal skills: `~/.claude/skills/<skill-name>/SKILL.md` (folder per skill; supporting files allowed; symlinked folders occur in practice — verified locally).
- Installed plugins: `~/.claude/plugins/installed_plugins.json` — plugin → `{scope, installPath, version, installedAt, lastUpdated, gitCommitSha}`.
- Marketplaces: `~/.claude/plugins/known_marketplaces.json` — name → source (github/directory) + install location.
- Plugin cache: `~/.claude/plugins/cache/<marketplace>/<plugin>/<version>/` with skills under `skills/` (arbitrary nesting, e.g. `skills/engineering/...`).
- Enabled/disabled: `enabledPlugins` map in `~/.claude/settings.json` (user), `.claude/settings.json` (project, shareable), `.claude/settings.local.json` (project-local). Per-project plugin enablement is supported (`claude plugin install --scope project`).

Sources: https://code.claude.com/docs/en/plugins-reference.md ("Plugin installation scopes"), https://code.claude.com/docs/en/discover-plugins.md ("Configure team marketplaces"). All file contents verified on this machine.

### Gaps / uncertainties (from the research agent)

1. No documented mechanism for per-plugin-skill disable at runtime; Skillet must build its own layer.
2. No query mechanism for "is this skill disabled" — inventory means parsing every SKILL.md's frontmatter.
3. No `claude skill disable` CLI — only plugin-level enable/disable.
4. Interaction of `disableBundledSkills` with plugin skills not fully specified in docs.
5. Plugin dependencies exist but have no skill-level override mechanism.

## Codex

Installed version at research time: codex-cli 0.141.0. Caveat: John's `~/.codex` install has many custom, non-stock additions (own tooling and experiments, possibly a canary channel); findings below are limited to what official docs or stock behavior confirm.

### Skills — a real mechanism, with per-skill disable built in

Codex has a genuine skills system, documented at https://developers.openai.com/codex/skills:

- Per-skill folder with `SKILL.md` (YAML frontmatter: `name`, `description`) plus optional `scripts/`, `references/`, `assets/`, and `agents/openai.yaml` (UI/policy metadata).
- **Multi-scope discovery**, scanned in priority order: repo `.agents/skills` (current/parent/root), user `$HOME/.agents/skills`, admin `/etc/codex/skills`, and system/legacy `$CODEX_HOME/skills` (`~/.codex/skills`, which also holds the bundled `.system/` skills). Verified locally: both `~/.agents/skills` and `~/.codex/skills` exist and are merged.
- **Auto-activation is real**: skills are invoked explicitly (`$skill-name`, `/skills`, selector) or *implicitly* — Codex autonomously selects skills whose `description` matches the task.

**Manual-only for Codex** = `agents/openai.yaml` → `policy: allow_implicit_invocation: false` (per-skill, official).

**Settings-level disable exists** (unlike Claude Code): `~/.codex/config.toml` carries `[[skills.config]]` entries with `enabled = false`, keyed by `path` (documented, https://developers.openai.com/codex/config-reference) or `name` (observed locally for namespaced plugin skills like `render:render-debug`; semantics inferred). Restart required to pick up changes. Verified locally at config.toml lines ~1688-1715.

### Custom prompts

`~/.codex/prompts/*.md` — plain markdown with YAML frontmatter (`description`, optional `argument-hint`), invoked as slash commands by filename; strictly user-invoked (no implicit invocation observed or documented). **Not documented on any official page the agent could reach** — the file convention is verified locally but invocation details are inferred. No per-prompt enable/disable found; presence = invocable.

### Plugins

Codex also has a live plugin system (`codex plugin add/list/marketplace/remove`; `plugin.json` bundling skills, MCP servers, commands, hooks — https://developers.openai.com/codex/plugins, github.com/openai/plugins). Plugins have their own `enabled = false` toggle in config.toml. The old `openai/skills` catalog repo is deprecated in favor of plugins.

### Gaps / uncertainties (from the research agent)

1. Custom prompts' invocation syntax, project-level scoping, and argument substitution are undocumented — inferred from files on disk.
2. The `[[skills.config]]` `name` key's exact semantics are inferred from the local file, not docs.
3. Docs-vs-binary drift possible: this install may be a canary channel; behavior on a fresh public install may differ in details.
4. Source-level confirmation (Rust code paths for prompt parsing and implicit matching) was blocked by GitHub rate limits.

## Consequences for Skillet (summary)

| Operation | Claude Code | Codex |
|---|---|---|
| Inventory | Parse `~/.claude/skills/`, plugin cache via `installed_plugins.json` | Parse `~/.codex/skills/`, `~/.agents/skills`, `~/.codex/prompts/`, config.toml overrides |
| Manual-only | Edit skill frontmatter (`disable-model-invocation: true`) — invasive, no alternative | Edit `agents/openai.yaml` (`allow_implicit_invocation: false`) |
| Per-skill disable without touching files | **Not supported** — Skillet must own suppress state and re-apply frontmatter edits across plugin updates | Supported natively via `[[skills.config]] enabled = false` in config.toml |
| Uninstall (archive) | Move skill folder out of source location | Move skill/prompt folder out; also clean stale config.toml entries |

The Claude Code suppress self-healing loop remains the single riskiest design element; the Codex side is straightforwardly file- and TOML-based.

## Re-verification against codex-cli 0.143.0 (issue #11, 2026-07-08)

The original Codex research (above) was done against John's local `~/.codex` install running codex-cli 0.141.0, flagged as a possible canary build. This section discharges that caveat: `@openai/codex@latest` (0.143.0 — newer than John's local 0.141.0, confirming this is genuinely a fresh public release, not the same binary) was installed into an isolated npm prefix with `CODEX_HOME`/`HOME` pointed at empty temp directories (no real install touched), and cross-checked against the two official docs pages already cited. Method: `strings` inspection of the compiled Rust binary (the real runtime — package `codex_core_skills`, modules `loader.rs`/`config_rules.rs`/`root_loader.rs`/`service.rs`) plus its bundled Python `skill-creator`/`init_skill.py` system-skill tooling, and a fresh fetch of https://developers.openai.com/codex/skills and https://developers.openai.com/codex/config-reference.

**Confirmed accurate, no drift:**
- `agents/openai.yaml` → `policy.allow_implicit_invocation` is exactly right: the Rust loader constructs `agent_yaml_path = skill_root / "agents" / "openai.yaml"` and defines `struct Policy`/`struct Interface` matching the documented shape. This is Skillet's Manual-only mechanism for Codex skills (`manual_only.go`) and is confirmed as the real, live mechanism — not stale.
- `[[skills.config]]` `path` and `enabled` are officially documented at https://developers.openai.com/codex/config-reference (`skills.config.<index>.path`, `skills.config.<index>.enabled`). `name` is still absent from that page — the existing "documented (`path`) vs. inferred (`name`)" distinction in this doc holds exactly as written; no change needed.
- `.system/` bundled skills (e.g. `imagegen`) really do live under `$CODEX_HOME/skills/.system/...`, confirmed directly in the binary's own bundled-skill instructions.
- Codex's skill-creator tooling *does* mention `disable-model-invocation`/`user-invocable` as SKILL.md frontmatter fields in its authoring guidance — but this string appears only in the bundled Python `skill-creator` script (author-facing guidance, likely for cross-tool compatibility with Claude Code skills), never in the Rust runtime's own `SkillFrontmatter`/`SkillMetadataFile` structs that actually govern Codex's activation behavior. This proactively closes a real ambiguity: Skillet correctly never writes these two fields to a *Codex* skill's own SKILL.md (Manual-only and Suppress both only touch `agents/openai.yaml`/`config.toml` for Codex) — that design is confirmed right, not incomplete.
- Custom prompts (`~/.codex/prompts/*.md`, `description`/`argument-hint` frontmatter) remain **completely absent** from both official docs pages, same as originally found — still inferred-from-observation only, not newly documented. `argument-hint` itself, however, is now also confirmed as a real, intentional field name (found verbatim in the skill-creator's own generation guidance for SKILL.md), lending indirect confidence to Skillet's prompt-parsing convention using the same field name.

**New findings — genuine drift/ambiguity, filed as follow-up issues rather than silently patched (per this issue's acceptance criteria):**
1. **`[[skills.config]]` entries with both `path` and `name`, or neither, are explicitly ignored by the real Rust validator** (`core-skills/src/config_rules.rs`: "ignoring skills.config entry without a path or name selector", "ignoring skills.config entry with both path and name selectors"). Skillet's own reader (`codexDisabledConfig.matches` in `codex.go`) doesn't replicate this exclusivity — a malformed entry with both keys set would still match by either field in Skillet, where real Codex would ignore it outright. Low-severity edge case (Skillet's own writers, `codex_suppress.go`/`archive.go`, never produce such an entry). Filed as issue #12.
2. **The current official docs' skill-discovery table lists only `.agents/skills` (REPO: `$CWD/.agents/skills`, `$CWD/../.agents/skills`, `$REPO_ROOT/.agents/skills`; USER: `$HOME/.agents/skills`), `/etc/codex/skills` (ADMIN), and a bundled SYSTEM scope — it does not list `~/.codex/skills` as a discovery location for user-authored (non-`.system`) skills at all.** This remains a docs/runtime mismatch, but issue #13 resolved the runtime side: on 2026-07-08, an authenticated local `codex-cli 0.141.0` session was run with two temporary probe skills, one at `~/.agents/skills/skillet-agents-root-probe-20260708/SKILL.md` and one non-`.system` skill at `~/.codex/skills/skillet-codex-root-probe-20260708/SKILL.md`. The command `codex exec --json --ephemeral --sandbox read-only --cd /Users/johnross/Projects/Skills\ Manager 'Do not run shell commands or inspect files. Answer only from skills that are already visible to you in the loaded Codex skill context...'` returned both expected probe phrases (`AGENTS_ROOT_VISIBLE_20260708` and `CODEX_ROOT_VISIBLE_20260708`) with no shell/tool-call items in the JSON stream. The run also emitted Codex's own warning that descriptions had been shortened but "Codex can still see every skill." Conclusion: for the current live Codex runtime on this machine, arbitrary user-authored skills under `$CODEX_HOME/skills` are still discovered, not just `.system/` bundled skills. Skillet's current dual-root scan (`AgentsHome/skills` and `CodexHome/skills`, `AgentsHome` taking priority on collision) remains empirically correct; no `internal/engine/codex.go` change is warranted from issue #13.
