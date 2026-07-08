# Skillet

Skillet is a terminal UI for taking control of agent Skills scattered across
Claude Code and Codex.

## Why

Skills pile up in more places than anyone can track by hand: your personal
`~/.claude/skills`, whatever plugins a marketplace dropped into
`~/.claude/plugins`, and Codex's own `~/.agents/skills` / `~/.codex/skills` /
`~/.codex/prompts`. There is no single place to see all of them, no easy way
to turn one off without editing YAML or TOML by hand, and — until now — no
way to remove one without it being gone for good. Skillet gives you one
inventory across both tools and a small set of reversible actions, so
tidying up your Skills doesn't mean risking one you'll want back later.

## Installation

Skillet's module is named `skillet` rather than a published import path, so
it isn't `go install`-able from a remote URL yet. Build it from a local
clone instead:

```
git clone https://github.com/jnyross/Skill-Manager.git
cd Skill-Manager
go install ./cmd/skillet
```

This installs a `skillet` binary to your `$GOPATH/bin` (or `$GOBIN`) — make
sure that directory is on your `PATH`. Alternatively, `go build -o skillet
./cmd/skillet` produces a local binary without installing it anywhere.

Requires Go 1.23 or later (see `go.mod`).

## Usage

Run:

```
skillet
```

Skillet resolves everything it needs from your home directory: Claude Code
state under `~/.claude`, Codex state under `~/.codex` and `~/.agents`, and
its own data (the Archive) under `~/.skillet`. There is nothing to configure.

### Main view

The main view lists every Skill Skillet found, grouped by Source —
**Personal**, **Plugin**, and **Codex** (Project is not supported in this
version). Each row shows the Skill's name, a truncated description, and its
current Activation state (Auto, Manual-only, Suppressed, or Disabled). A
Codex custom prompt is labeled `[prompt]`. A Plugin skill's row shows "one
of N in `<plugin-name>`" so you can see how many other Skills would be
affected by removing that plugin. If any Skills couldn't be read (a
malformed file, a missing directory, a plugin whose install path vanished),
a Notices section at the bottom explains what was skipped and why — this
never blocks the rest of the list.

Keys:

| Key | Action | Applies to |
|---|---|---|
| `up` / `k`, `down` / `j` | Move the cursor | everywhere |
| `u` | Archive the selected Skill | Personal, Codex (skills and prompts) |
| `s` | Suppress / un-suppress the selected Skill | Plugin skills, Codex skills |
| `m` | Toggle Manual-only / Auto-activation | Personal skills, Codex skills (not prompts) |
| `x` | Uninstall the selected Skill's whole plugin | Plugin skills |
| `a` | Switch to the Archive view | — |
| `q` / `ctrl+c` | Quit | — |

Pressing a key for an action that doesn't apply to the selected row (for
example `x` on a Personal skill) shows a status message explaining why,
instead of doing anything.

### Archive view

Press `a` from the main view to see everything currently archived: name,
original Source, original location, and when it was archived.

| Key | Action |
|---|---|
| `up` / `k`, `down` / `j` | Move the cursor |
| `r` | Restore the selected entry |
| `p` | Purge the selected entry |
| `a` / `esc` | Back to the main view |
| `q` / `ctrl+c` | Quit |

### Confirmation model

Every action that changes something on disk asks for confirmation first:
Skillet shows a one-line description of exactly what it's about to do and
waits for a keypress. Press `y` to proceed; any other key cancels and
changes nothing. Browsing the list, moving the cursor, and switching between
the main and Archive views never changes anything by themselves — only a
confirmed `y` does.

## Actions

Skillet's actions follow the same vocabulary throughout the UI and this
document:

- **Archive** — the manager-owned holding area where uninstalled skills go
  instead of being deleted. Skills in the archive are invisible to Claude
  Code and Codex but fully recoverable. Pressing `u` on a Personal or Codex
  skill (or Codex prompt) archives it, after confirmation.
- **Restore** — returning an archived skill to its original source, exactly
  as it was (including recreating a symlink if that's how it was installed).
  Pressing `r` in the Archive view restores the selected entry.
- **Purge** — permanently deleting a skill from the archive. This is the
  only destructive operation in Skillet. Pressing `p` in the Archive view
  purges the selected entry; there is no way to undo it.
- **Manual-only** — the state of a skill whose Auto-activation is off; it
  runs only when the user explicitly invokes it, never on the model's own
  judgment. It is not the same as being disabled — the skill still works,
  it just won't trigger itself. Pressing `m` on a Personal or Codex skill
  toggles Manual-only on or off. Not available for Codex custom prompts
  (which have no auto-activation to turn off) or Plugin skills.
- **Suppress** — hiding a single skill from the model and slash commands
  while leaving what it belongs to installed and intact. Pressing `s` does
  this, but the mechanism differs by Source:
  - On a **Plugin skill**, Suppress is Skillet's own mechanism: the
    per-skill alternative to uninstalling the whole plugin. Skillet owns and
    maintains this state, and re-applies it automatically if the plugin is
    updated (the plugin itself is never touched or removed).
  - On a **Codex skill**, Suppress writes to Codex's own native
    `config.toml` disable setting — the same mechanism Codex itself uses.
    This takes effect after you restart Codex; Skillet tells you so in its
    status line.

  Pressing `s` again on a Suppressed (or natively Disabled) skill
  un-suppresses it, using the matching mechanism for its Source.
- **Uninstall plugin** — pressing `x` on any skill belonging to a plugin
  removes that plugin entirely, taking every skill it bundles with it. The
  confirmation lists every skill that will go. This is the alternative to
  Suppress when you want a plugin gone rather than just one of its skills
  hidden — plugin skills can't be individually uninstalled, only suppressed
  or removed as part of the whole plugin.

## Scope

This version covers Personal, Plugin, and Codex skills and Codex custom
prompts, all at the user level. Project skills (installed inside a single
repository) are not yet supported.
