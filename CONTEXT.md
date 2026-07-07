# Skillet

A TUI (command: `skillet`) for seeing every agent skill installed across Claude Code and Codex, and acting on them: uninstalling them or stopping them from auto-activating.

## Language

**Skill**:
Any installable unit of agent instructions the manager governs, regardless of host tool — Claude Code skills, plugin-bundled skills, and Codex prompts alike.
_Avoid_: command, prompt (as an umbrella term), extension

**Source**:
Where a skill is installed from and lives — one of Personal, Project, Plugin, or Codex. Determines what uninstall and activation control mean for that skill.
_Avoid_: location, origin, type

**Personal skill**:
A skill installed at the user level of Claude Code, following the user into every session.

**Project skill**:
A skill installed inside a single repository, applying only there.

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

**Shortlist**:
The user-curated set of skills and plugins preauthorised for seeding into new projects. Only shortlisted items are offered during seeding.
_Avoid_: loadout, template, favourites

**Seeding**:
Setting up a new project's skills by picking from the shortlist one item at a time — copying chosen skills into the project and enabling chosen plugins for it. A one-time copy; the project owns the result with no ongoing sync.
_Avoid_: sync, install (ambiguous with adding to the user level)
