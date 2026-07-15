# Skillet

A TUI (command: `skillet`) for seeing every agent skill installed across Claude Code and Codex, and acting on them: uninstalling them or stopping them from auto-activating.

## Language

**Skill**:
Any installable unit of agent instructions the manager governs, regardless of host tool — Claude Code skills, plugin-bundled skills, and Codex prompts alike.
_Avoid_: command, prompt (as an umbrella term), extension

**Source**:
Where a skill is installed from and lives — one of Personal, Plugin, Codex, or Project. Personal, Plugin, and Codex are all user-level, one per tool/mechanism. Project is the one repo-level group and spans skills from either tool (a Claude Code project skill and a Codex repo-scoped skill are both "Project", distinguished by their **Tool**). Determines what uninstall and activation control mean for that skill.
_Avoid_: location, origin, type

**Tool**:
Which underlying system governs a skill — Claude Code or Codex. Orthogonal to Source: every skill has exactly one of each. Only surfaced as a visible label within the Project group, where both tools' repo-scoped skills sit side by side; Personal/Plugin skills are always Claude Code and the Codex group is always Codex, so the label would be redundant there.
_Avoid_: type (already used for Kind), platform

**Tool Adapter**:
Skillet's capability contract for configuring and verifying one Tool. An Adapter makes Tool-specific behavior explicit without introducing Harness as a synonym for Tool.
_Avoid_: Harness Adapter, provider, integration

**Personal skill**:
A skill installed at the user level of Claude Code, following the user into every session.

**Project skill**:
A skill installed inside a single repository, applying only there — either a Claude Code skill under `<repo>/.claude/skills` or a Codex skill under `<repo>/.agents/skills` (discovered via the repo-root walk-up from the current working directory, mirroring Codex's own repo-scope discovery). Grouped as one Source, tagged with its Tool.

**Plugin skill**:
A skill that arrives bundled inside a Claude Code plugin from a marketplace; it cannot be removed alone without affecting its plugin.

**Codex skill**:
A prompt or skill installed at the user level of Codex.

**Auto-activation**:
A skill's ability to be invoked by the model on its own judgment, based on the skill's description. The manager can turn this off per skill.
_Avoid_: auto-trigger, model-invocation (in user-facing copy)

**Manual-only**:
The state of a skill whose auto-activation is off; it runs only when the user explicitly invokes it by slash command.
_Avoid_: disabled (which implies it can't run at all)

**Archive**:
The manager-owned holding area where uninstalled skills go instead of being deleted. Skills in the archive are invisible to Claude Code and Codex but fully recoverable.
_Avoid_: trash, backup

**Restore**:
Returning an archived skill to its original source, exactly as it was.

**Purge**:
Permanently deleting a skill from the archive. The only destructive operation in the manager.
_Avoid_: delete (ambiguous with uninstall)

**Suppress**:
Hiding a single plugin skill from the model and slash commands while leaving its plugin installed and intact. The per-skill alternative to uninstalling a whole plugin; Skillet owns and maintains this state across plugin updates.
_Avoid_: uninstall (plugin skills can't be individually uninstalled), block

**Library**:
The user's own catalog of skills and plugins they maintain, spanning all three user-level Sources (Personal, Plugin, Codex) — never Project, since a Project skill already lives in a specific repo. Each entry carries an install-source descriptor (a skills.sh `owner/repo` reference, a git URL, a Claude/Codex marketplace pointer, or a local filesystem path) rather than a frozen copy, so installing an entry always resolves its current, latest version from that source.
_Avoid_: Shortlist (superseded — the old name implied a pre-approval flag, not a maintained source-of-truth catalog), loadout, favourites

**Built-in catalog**:
The versioned, Skillet-maintained set of source descriptors approved for guided setup. It proposes trusted entries to add or Install; it is not the user's Library and never silently changes it.
_Avoid_: marketplace, Library, internet catalog

**Bundle**:
A named group of Library entries for quick, repeatable setup. Each member stores its own remembered Activation preference (Auto or Manual-only) independent of the entry's default — the same Library skill can be Auto in one Bundle and Manual-only in another. Installable to either Personal or a specific repo, same target choice as installing a single Library entry.
_Avoid_: template, loadout

**Install**:
Resolving a Library entry (or every member of a Bundle) from its install-source descriptor and placing it at a chosen target — Personal or a specific repo — applying any Bundle-remembered Activation preference along the way. The one general action for getting a Library/Bundle item onto a machine or into a repo, at any time, not just new-project setup.
_Avoid_: Seeding (superseded — implied a one-time, new-project-only flow; Install applies uniformly, any time), sync

**Agent-ready workspace**:
A project folder prepared for Claude Code and Codex with shared agent instructions, approved skills, and enough metadata for both Tools to discover the project. It contains no generated application code.
_Avoid_: app template, framework starter, scaffold

**Managed file**:
A workspace path that a prior Skillet setup explicitly created or adopted and may therefore update or roll back subject to conflict checks. Existing user-owned paths are never Managed files implicitly.
_Avoid_: generated file (does not establish ownership), owned file

**Workspace receipt**:
The durable record of a guided setup's Managed files, source catalog version, content identity, and outcome. It lets a repeat setup distinguish Skillet-managed state from user work.
_Avoid_: Install receipt, lockfile, manifest

**Setup outcome**:
The result of guided workspace setup — Blocked, Configured-unverified, Verified, or Partial. Partial is reserved for an external Tool change that remains after Skillet rolls back its own reversible writes.
_Avoid_: success flag, warning, status

**Release**:
A published, versioned build of Skillet that people can install without cloning the source repository or having the Go toolchain. A Release is the common source from which every supported Distribution channel obtains the same Skillet version.
_Avoid_: build (a build is not necessarily published), deployment

**Distribution channel**:
A supported route by which a person installs and later upgrades Skillet on a machine. The first public channel is the scoped npm package selected by ADR 0006; later channels must define their own same-channel Upgrade contract.
_Avoid_: marketplace, source (already means where a managed Skill lives)

**Upgrade**:
Replacing an installed Skillet Release with a newer Release through the same Distribution channel that installed it. Upgrade does not mean Skillet rewrites its own executable.
_Avoid_: self-update, automatic update

**Supported platform**:
An operating-system and CPU combination inside Skillet's stated compatibility contract. The first public set is macOS 12 or newer and 64-bit Linux kernel 3.2 or newer, each on native amd64 or arm64.
_Avoid_: any machine, universal (both hide real compatibility boundaries)

**Tested platform**:
A Supported platform environment exercised continuously by Skillet's release verification. Tested platforms are evidence points inside the broader support contract, not the complete list of machines on which Skillet may run.
_Avoid_: supported platform (the contract and its test samples are different)

**Install receipt**:
The durable record proving that a direct Distribution channel installation is owned by Skillet's first-party installer. It lets a later Upgrade distinguish that installation from one owned by a package manager or another program.
_Avoid_: lockfile, manifest, Homebrew receipt
