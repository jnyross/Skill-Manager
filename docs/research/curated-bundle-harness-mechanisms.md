# Curated Bundle harness mechanisms

Primary-source research for [issue #48](https://github.com/jnyross/Skill-Manager/issues/48), extending the existing evidence in `skill-mechanisms.md` and `library-install-mechanisms.md` before the Harness Adapter design.

Researched 2026-07-15 against official documentation, the Agent Skills specification, the OpenAI Codex `rust-v0.144.1` source, local CLI help, and isolated scratch probes. Local versions were Claude Code **2.1.207** and Codex CLI **0.144.1**. No real Claude Code or Codex configuration, skill tree, plugin cache, credentials, or project files were changed.

## Status

The direct-skill mechanisms needed by the guided setup are sufficiently verified for both harnesses at Personal and Project scope. Claude Code plugins are also scriptable at Personal and Project scope. Codex plugins are scriptable at Personal scope, but the current CLI has no Project install scope: repository marketplaces are discoverable from a repo, while installed plugin bytes and enablement remain user-owned under `CODEX_HOME`.

Two current-runtime facts supersede older assumptions:

1. Codex 0.144.1 walks **every directory from the repository root to the current working directory** for `.agents/skills`; it no longer implements the three-candidate-only rule recorded by issue #19.
2. Claude Code 2.1.207 has a `skillOverrides` setting that can hide a direct skill without editing its `SKILL.md`. Frontmatter remains the portable way for a Bundle-installed copy to carry its own Auto or Manual-only preference.

The Codex ancestor walk was also confirmed by an authenticated, read-only live
runtime probe on 2026-07-15: from a working directory at `<repo>/a/b/c`, Codex
loaded a uniquely named probe skill placed at `<repo>/a/.agents/skills`, which
is neither the current directory, its immediate parent, nor the repository
root. The probe prohibited shell and tool use, and `codex exec --json
--ephemeral --sandbox read-only` returned the skill's exact sentinel in an
`agent_message`. This closes the runtime-verification gap for the adapter and
inventory contract.

## Evidence labels

- **Documented** — stated in current first-party documentation or the Agent Skills specification.
- **Source-verified** — confirmed in the tagged source for the locally installed release.
- **Observed** — reproduced locally in an isolated temporary home/project.
- **Inferred** — a design implication from the evidence; not a harness contract by itself.

## Capability summary

| Mechanism | Claude Code | Codex | Evidence and constraint |
|---|---|---|---|
| Direct skill, Personal | `~/.claude/skills/<name>/` | `$HOME/.agents/skills/<name>/` | Documented; direct filesystem placement |
| Direct skill, Project | `<repo>/.claude/skills/<name>/` | `<repo>/.agents/skills/<name>/` | Documented; distinct roots must remain distinct |
| Create missing skill directories | Yes | Yes | Direct installer can create parents before atomic placement; running sessions may need restart |
| Auto activation | Default | Default | Description is available for implicit selection |
| Manual-only | `disable-model-invocation: true` in `SKILL.md` | `agents/openai.yaml` with `policy.allow_implicit_invocation: false` | Documented, harness-specific overlays |
| Fully disable one direct skill | `skillOverrides.<name> = "off"` | `[[skills.config]]`, `path`, `enabled = false` | Documented; distinct from Manual-only |
| Non-interactive runtime probe | `claude -p ...` | `codex exec ...` | Documented; requires executable and working authentication/provider |
| Plugin install, Personal | `claude plugin install ... --scope user` | `codex plugin add ... --json` | Documented/help/observed |
| Plugin install, Project | `claude plugin install ... --scope project` | **Not supported by the current CLI** | Codex repo marketplaces are not project-scoped installed plugins |
| Plugin enable | `claude plugin enable ... --scope ...` | Install enables in user `config.toml`; interactive `/plugins` can toggle | Codex has no `plugin enable` CLI subcommand in 0.144.1 |
| Machine-readable plugin check | `claude plugin list --json` | `codex plugin list --json` | CLI help/observed |
| Harness health check | `claude doctor`, `claude auth status --json` | `codex doctor --json`, `codex login status` | CLI help/observed |

## Shared-compatible content and harness overlays

Both products say their skills build on the [Agent Skills open standard](https://agentskills.io/specification). The portable core is:

- a directory whose required entry point is `SKILL.md`;
- YAML frontmatter containing a valid `name` and non-empty `description`;
- Markdown instructions;
- optional `scripts/`, `references/`, and `assets/` directories.

The standard's `compatibility` field is the right place for source-authored environmental requirements such as required executables, network access, or an intended product. Its `allowed-tools` field is experimental and support may differ between clients. A Harness Adapter must therefore validate actual dependencies and tool names rather than treating standard-shaped content as behaviorally portable.

The safe Bundle representation is **shared core plus per-harness overlay**, not one blindly copied tree:

- Claude Code activation belongs in `SKILL.md` frontmatter (`disable-model-invocation`). Claude-only fields such as `context`, `agent`, dynamic shell substitution, hooks, and Claude tool names must be marked Claude-specific.
- Codex activation and dependency metadata belong in `agents/openai.yaml`. Its `policy.allow_implicit_invocation` and `dependencies.tools` fields are Codex-specific.
- A skill body or script that refers to a harness-specific tool, environment variable, invocation syntax, plugin namespace, connector, or permission model is not cross-harness merely because its folder satisfies Agent Skills.
- Claude and Codex plugin packages are different formats: Claude uses `.claude-plugin/plugin.json`; Codex uses `.codex-plugin/plugin.json`. A marketplace entry for one must not be advertised as installable in the other unless an explicit target-specific artifact has been verified.

**Adapter implication (inferred):** a Bundle member should select an artifact/recipe by Harness and target capability. The adapter may reuse identical core bytes when compatibility is proven, then render only that harness's activation overlay into its own destination.

## Claude Code

### Direct skill placement and discovery

The [Claude Code skills documentation](https://code.claude.com/docs/en/skills#where-skills-live) defines:

| Scope | Destination |
|---|---|
| Personal | `~/.claude/skills/<skill-name>/SKILL.md` |
| Project | `<repo>/.claude/skills/<skill-name>/SKILL.md` |

Project discovery walks from the starting directory through **every parent directory to the repository root**. Claude Code can also discover nested `.claude/skills` directories below the starting directory when it works on files there. This remains different from a simple repo-root-only inventory.

Direct placement has no required Claude CLI command. A non-interactive adapter can:

1. resolve the source into scratch storage;
2. validate the complete skill directory;
3. create the destination parent if absent;
4. atomically replace or move the validated directory according to Skillet's conflict policy;
5. re-read the placed copy and activation state.

The docs explicitly show `mkdir -p` for the Personal skill directory, so a missing `~/.claude/skills` or project `.claude/skills` directory is not an error. [Claude Code watches existing skill roots](https://code.claude.com/docs/en/skills#live-change-detection), but if a top-level skills directory did not exist when a session started, that session must be restarted before it can watch the new directory. A fresh verification session avoids this ambiguity.

### Auto, Manual-only, and disabled

For a direct skill:

- Auto is the default: `disable-model-invocation` absent or `false`.
- Manual-only is `disable-model-invocation: true`. Claude removes its description from model-visible context but preserves explicit `/skill-name` invocation. This is Skillet's Manual-only state. [Official frontmatter reference](https://code.claude.com/docs/en/skills#control-who-invokes-a-skill).
- Fully hidden is a `skillOverrides` value of `"off"`. `"user-invocable-only"` is the settings-level equivalent of Manual-only, while `"on"` and `"name-only"` keep the skill available in different ways. The `/skills` UI writes overrides to `.claude/settings.local.json`; plugin skills are explicitly excluded from this mechanism. [Official visibility table](https://code.claude.com/docs/en/skills#override-skill-visibility-from-settings).

For a Bundle-installed disconnected copy, editing its frontmatter remains the most deterministic way to carry member Activation across machines and scopes. A named `skillOverrides` entry may affect another same-named skill and introduces settings ownership/precedence concerns. Static validation must still inspect effective overrides because `"off"` can make a correctly placed skill unavailable.

### Plugin installation and enablement

Claude plugins use a mandatory two-stage setup when the marketplace is not already known:

```text
claude plugin marketplace add <source> --scope user|project|local
claude plugin install <plugin>@<marketplace> --scope user|project|local
claude plugin enable <plugin>@<marketplace> --scope user|project|local
```

The first two commands and their scope behavior are documented in the [plugin reference](https://code.claude.com/docs/en/plugins-reference#cli-commands-reference). The install scope determines which settings file records enablement:

| Scope | Settings file |
|---|---|
| `user` | `~/.claude/settings.json` |
| `project` | `<repo>/.claude/settings.json` |
| `local` | `<repo>/.claude/settings.local.json` |

The prior isolated 2.1.201 probe in `library-install-mechanisms.md` established that `--scope project` creates a missing `.claude/settings.json`, while plugin bytes remain in the user cache. Current 2.1.207 help exposes the same commands and flags. Installation normally enables the plugin, but the explicit `plugin enable` step gives the adapter a stable postcondition and covers disabled/default-disabled state.

`claude plugin list --json` is the machine-readable installed/enabled check. `claude plugin details <id>` provides the component inventory, and `claude plugin validate --strict <path>` validates a source plugin or marketplace manifest before installation. A successful install is not proof that plugin MCP servers, hooks, user configuration, or external authentication are ready.

### Claude post-install validation

Static validation, with no model call:

1. Destination is under the correct Personal or resolved Project root.
2. `SKILL.md` parses and contains the expected identity/description.
3. Placed bytes match the resolved source plus the declared Claude overlay.
4. Manual-only/Auto state matches frontmatter and no effective `skillOverrides` entry makes it unavailable.
5. For a plugin, `claude plugin list --json` reports the expected id installed and enabled at the intended scope; `plugin details` contains the expected components.

Runtime readiness, when the executable and authentication are available:

```text
claude -p --no-session-persistence --output-format json \
  --tools "" --disallowedTools "mcp__*" \
  "/<skill-name> Return only <member-specific-sentinel>."
```

Run from the actual target project for Project scope. Use a curated, no-side-effect verification prompt; relax the tool restrictions only when the member's declared verification genuinely requires a tool. `claude -p` and `--no-session-persistence` are documented in the [CLI reference](https://code.claude.com/docs/en/cli-reference). A returned sentinel proves explicit discovery and loading, not the quality of implicit activation. Implicit behavior needs separate should-trigger/should-not-trigger eval prompts in fresh sessions, as Anthropic's skill docs recommend.

Preflight can use `claude auth status --json` and read-only `claude doctor`. `doctor` validates installation and settings but does not enumerate direct skills, so it cannot replace the member probe.

## Codex

### Direct skill placement and current discovery behavior

The current [Codex skills documentation](https://learn.chatgpt.com/docs/build-skills#where-to-save-skills) defines:

| Scope | Destination |
|---|---|
| Personal | `$HOME/.agents/skills/<skill-name>/SKILL.md` |
| Project | `<repo or ancestor>/.agents/skills/<skill-name>/SKILL.md` |

The public page currently says Codex scans `.agents/skills` in every directory from the current working directory to the repository root, although its adjacent table still illustrates only CWD, CWD's parent, and repo root. The implementation resolves the ambiguity.

In `rust-v0.144.1`, [`repo_agents_skill_roots`](https://github.com/openai/codex/blob/rust-v0.144.1/codex-rs/core-skills/src/loader.rs#L375-L402) obtains every directory between the detected project root and CWD, and [`dirs_between_project_root_and_cwd`](https://github.com/openai/codex/blob/rust-v0.144.1/codex-rs/core-skills/src/loader.rs#L463-L481) returns the full inclusive ancestor chain. Missing `.agents/skills` directories are ignored. This is source-verified current behavior and supersedes the three-fixed-candidate conclusion recorded for 0.141.0.

Codex still reads the deprecated `$CODEX_HOME/skills` user location for backward compatibility in 0.144.1, but new curated Personal installs should target the documented `$HOME/.agents/skills`. That preserves the project's existing distinction between inventory compatibility and preferred installation target.

As with Claude, no Codex executable is required to place a direct skill. The adapter creates missing parent directories, installs atomically, and validates the resulting tree. Codex docs say skill changes are detected automatically; if a change does not appear, restart Codex. A fresh verification process is therefore the clean completion boundary.

### Auto, Manual-only, and disabled

- Auto is the default when `agents/openai.yaml` omits `policy.allow_implicit_invocation` or sets it to `true`.
- Manual-only sets `policy.allow_implicit_invocation: false`; explicit `$skill-name` invocation still works.
- Fully disabled uses a `[[skills.config]]` entry in `~/.codex/config.toml` with the skill path and `enabled = false`, followed by restart. [Official skill configuration](https://learn.chatgpt.com/docs/build-skills#enable-or-disable-skills) and [config reference](https://learn.chatgpt.com/docs/config-file/config-reference#skillsconfig).

Manual-only and disabled must not be conflated. A placed skill whose path is disabled in effective config is configured on disk but not ready for either implicit or explicit use.

### Plugins: Personal installation only in the current CLI

Codex plugins use `.codex-plugin/plugin.json`, not Claude's manifest. The [Codex plugin authoring docs](https://learn.chatgpt.com/docs/build-plugins) define Personal and repo marketplace files:

- Personal catalog: `~/.agents/plugins/marketplace.json`.
- Repo catalog: `$REPO_ROOT/.agents/plugins/marketplace.json`.

A repo marketplace makes entries discoverable in that repository; it does **not** make `codex plugin add` a Project-scoped install. Current non-interactive commands are:

```text
codex plugin marketplace add <source> [--ref <ref>] [--sparse <path>] --json
codex plugin add <plugin>@<marketplace> --json
codex plugin list --json
```

Observed with an isolated `HOME`/`CODEX_HOME` and local marketplace on Codex 0.144.1:

1. `marketplace add` returned `marketplaceName`, `installedRoot`, and `alreadyAdded` as JSON.
2. `plugin list --available --json` reported the local plugin as `installed: false`, `enabled: false`.
3. `plugin add ... --json` copied the plugin to `$CODEX_HOME/plugins/cache/<marketplace>/<plugin>/<version>`.
4. It created a missing `$CODEX_HOME/config.toml` and wrote:

   ```toml
   [plugins."probe-plugin@skillet-issue48-local"]
   enabled = true
   ```

5. A following `plugin list --json` reported `installed: true`, `enabled: true`.
6. No Project config or project-local plugin install was created, even though the command ran from a scratch project directory.

This is also source-verified: the installer enables the completed install with [`set_user_plugin_enabled`](https://github.com/openai/codex/blob/rust-v0.144.1/codex-rs/core-plugins/src/manager.rs#L1427-L1501), and the config editor always writes `$CODEX_HOME/config.toml` ([`plugin_edit.rs`](https://github.com/openai/codex/blob/rust-v0.144.1/codex-rs/config/src/plugin_edit.rs#L21-L102)). Codex 0.144.1 has no `codex plugin enable` subcommand; the interactive `/plugins` browser can toggle an installed plugin, and direct user-config editing is the underlying mechanism.

The official user docs require a **new session** after plugin installation before bundled skills or tools are available. Plugin connectors, apps, MCP servers, and hooks can have additional authentication, policy, or trust requirements; installation and `enabled = true` do not prove those capabilities ready. In particular, Codex skips plugin hooks until the user reviews and trusts them.

**Adapter implication (inferred):** advertise Codex `plugin.install.personal`, not `plugin.install.project`. A Project Bundle that promises both harnesses must use a direct Codex project skill or another explicitly verified Codex Project artifact; it cannot treat a user-installed Codex plugin as Project-scoped.

### Codex post-install validation

Static validation:

1. Confirm the direct skill is at the selected `.agents/skills` root and parses as an Agent Skill.
2. Confirm its `agents/openai.yaml` Activation and that no effective `skills.config` rule disables the placed path.
3. For plugins, parse `codex plugin list --json` and require the expected id, version/source, `installed: true`, and `enabled: true`.
4. Treat declared MCP/app/tool dependencies and hook trust as separate readiness checks.

Runtime readiness:

```text
codex exec --json --ephemeral --sandbox read-only -C <target-project> \
  "Use $<skill-name>. Do not run tools. Return only <member-specific-sentinel>."
```

`codex exec` and `--ephemeral` are documented in [Non-interactive mode](https://learn.chatgpt.com/docs/non-interactive-mode). The explicit `$skill-name` mention is the documented invocation form. Parse the JSONL stream and require a normal completed response containing only the sentinel, with no tool calls. An implicit-activation check is a separate prompt without `$skill-name`.

`codex doctor --json` is a useful preflight because it returns redacted machine-readable checks for runtime, config, authentication, network, and installation health. In an isolated unauthenticated home it correctly reported config load as healthy while authentication failed, demonstrating why static configuration and runtime readiness must be separate statuses. `codex login status` is a narrower authentication check.

## Missing executable and configured-unverified semantics

Issue #44 explicitly excludes installing or authenticating harness executables. The adapter should separate operations that can complete without a harness binary from operations owned by a harness CLI.

| Situation | Evidence available | Result implication |
|---|---|---|
| Direct skills installed; harness executable absent | Validated files, paths, activation, no conflicting disable rule | **Configured, unverified** for that harness |
| Direct skills installed; executable present but no usable auth/provider | Static checks plus failed auth/doctor | **Configured, unverified**, with authentication/provider reason |
| Direct skills installed; fresh explicit invocation succeeds | Static checks plus sentinel response | **Verified** for explicit discovery/use |
| Claude plugin CLI absent | Skillet cannot safely maintain Claude's marketplace/cache/install records | Member is **blocked**, not configured |
| Codex plugin CLI absent | Skillet cannot safely maintain Codex's marketplace/cache/user config | Member is **blocked**, not configured |
| Plugin installed/enabled but connector/MCP auth absent | Plugin inventory passes; capability auth does not | Plugin configured; dependent capability unverified/blocked |
| One harness/member fails after others complete | Per-member receipts show mixed outcomes | **Partially completed**, never all-ready |

The final status vocabulary belongs to issue #51, but issue #48 establishes the evidence boundary: “configured” is a deterministic filesystem/config postcondition; “verified” requires a fresh real harness process on the actual target; and a CLI-owned plugin cannot be called configured when its required executable was absent.

## Harness Adapter capability implications

An adapter should declare capabilities rather than exposing a generic “install” promise:

- `skill.install.personal`
- `skill.install.project`
- `skill.activation.auto`
- `skill.activation.manual_only`
- `skill.disable.detect`
- `plugin.marketplace.configure`
- `plugin.install.personal`
- `plugin.install.project`
- `plugin.enable`
- `validate.static`
- `validate.runtime.explicit`
- `validate.runtime.implicit`
- `health.executable`
- `health.authentication`
- `dependency.connector`, `dependency.mcp`, and `dependency.hook_trust` as applicable

Each recipe should also declare:

- whether an executable is required to configure it;
- target roots/settings it may write;
- whether it can create missing directories/files;
- restart/new-session requirements;
- conflict and managed-file ownership behavior;
- the exact static evidence and optional runtime probe that closes the operation.

Current capability matrix:

| Capability | Claude Code 2.1.207 | Codex 0.144.1 |
|---|---:|---:|
| Direct skill Personal/Project | Yes / Yes | Yes / Yes |
| Auto/Manual-only | Yes / Yes | Yes / Yes |
| Plugin Personal/Project | Yes / Yes | Yes / **No** |
| Plugin enable non-interactively | Yes | Install enables; no separate CLI command |
| Missing direct-skill roots created by adapter | Yes | Yes |
| Static member verification without executable | Yes | Yes |
| Runtime member verification without executable/auth | No | No |

## Required post-install evidence

For every Bundle member, retain a result record containing at least:

- Bundle/member identity and Harness;
- Personal or Project target and canonical destination;
- resolved source descriptor and source revision/hash when the resolver exposes one;
- installed artifact kind and adapter recipe;
- Activation requested and Activation observed;
- static validation result with placed content hash;
- executable/version/auth preflight result;
- runtime probe status and reason when skipped;
- plugin id/version/source, enabled state, and unresolved dependency/trust checks;
- restart/new-session requirement;
- warnings, conflict decision, and rollback/partial-completion state.

This is operation evidence, not background drift tracking; it does not change ADR 0004's no ongoing version tracking decision.

## Unresolved gaps and follow-ups

1. **Codex Project plugin is unsupported by the current CLI.** Decide in issue #47 whether such a member is invalid, downgraded to Personal with explicit consent, or replaced by a Project direct-skill artifact. Do not silently relabel it.
2. **Plugin Activation semantics are not Bundle skill Activation.** Auto/Manual-only maps cleanly to individual direct skills. Enabling/disabling a whole plugin affects all bundled components; issue #47 must model that separately.
3. **No harness exposes a complete non-interactive direct-skill inventory command.** Static parsing plus an authenticated fresh-session probe remains the strongest available evidence.
4. **Claude `skillOverrides` precedence needs fixture coverage.** The docs define states and the local-settings UI, but Skillet should test user/project/local precedence before it manages overrides directly.
5. **Runtime probes cost a model call and may expose member side effects.** Curated members need a declared safe explicit verification prompt; implicit-activation quality belongs in the release/eval matrix, not every install.
6. **Plugins can be configured but capability-unready.** Connector auth, MCP startup/auth, required userConfig, and hook trust need member-specific checks. A generic plugin-installed check cannot claim them ready.
7. **Claude plugin failure exit codes previously varied.** Keep the existing completion-marker plus readback verification until a current regression test proves exit status alone reliable.

## Probe record

- Read-only local help/version checks:
  - `claude --version` → `2.1.207 (Claude Code)`.
  - `codex --version` → `codex-cli 0.144.1`.
  - Claude help confirmed `plugin install`, `plugin enable`, marketplace `add`, `plugin list --json`, `plugin details`, `plugin validate --strict`, `doctor`, and print-mode flags.
  - Codex help confirmed `plugin marketplace add/list`, `plugin add/list --json`, `doctor --json`, `login status`, and `exec --json --ephemeral`; `codex plugin enable` was rejected as an unknown subcommand.
- Isolated Codex home/project probe under `/tmp/skillet-issue48`:
  - empty marketplace and plugin lists returned valid JSON;
  - `doctor --json` separated healthy config loading from missing authentication;
  - a local `.codex-plugin` marketplace installed non-interactively, created missing user config/cache paths, enabled the plugin, and returned machine-readable installed/enabled state;
  - no real `~/.codex`, `~/.agents`, or project files were touched.
- Authenticated Codex ancestor-discovery probe under a disposable
  `/private/tmp/skillet-codex-probe.*` repository:
  - working directory was `<repo>/a/b/c`;
  - the only probe skill was `<repo>/a/.agents/skills/skillet-ancestor-probe`,
    deliberately outside the old three fixed candidates;
  - `codex exec --json --ephemeral --sandbox read-only` was instructed not to
    use shell commands or tools and returned the exact skill sentinel;
  - the disposable repository was removed after the probe.
- Isolated Claude home probe under `/tmp/skillet-issue48`:
  - `claude doctor` was read-only and reported the intentionally missing authentication/config state;
  - `claude plugin list --json` returned an empty JSON list;
  - no real `~/.claude` files were touched.

## Primary sources

- [Agent Skills specification](https://agentskills.io/specification)
- [Claude Code skills](https://code.claude.com/docs/en/skills)
- [Claude Code plugin reference](https://code.claude.com/docs/en/plugins-reference)
- [Claude Code CLI reference](https://code.claude.com/docs/en/cli-reference)
- [Codex skills](https://learn.chatgpt.com/docs/build-skills)
- [Codex plugins](https://learn.chatgpt.com/docs/plugins)
- [Build Codex plugins](https://learn.chatgpt.com/docs/build-plugins)
- [Codex config reference](https://learn.chatgpt.com/docs/config-file/config-reference)
- [Codex non-interactive mode](https://learn.chatgpt.com/docs/non-interactive-mode)
- [Codex 0.144.1 skill loader](https://github.com/openai/codex/blob/rust-v0.144.1/codex-rs/core-skills/src/loader.rs)
- [Codex 0.144.1 plugin manager](https://github.com/openai/codex/blob/rust-v0.144.1/codex-rs/core-plugins/src/manager.rs)
- [Codex 0.144.1 plugin config editor](https://github.com/openai/codex/blob/rust-v0.144.1/codex-rs/config/src/plugin_edit.rs)
