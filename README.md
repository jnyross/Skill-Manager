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

The first public Distribution channel is the scoped npm package:

```sh
npm install --global @jnyross/skillet@next
skillet --version
skillet
```

```
git clone https://github.com/jnyross/Skill-Manager.git
cd Skill-Manager
go build -o skillet ./cmd/skillet
```

The npm channel requires Node.js 22.14 or newer and npm 10.9 or newer. It
installs a small JavaScript launcher and one native binary selected for macOS
or Linux on arm64 or x64; it runs no install lifecycle scripts. Upgrade through
the same channel with `npm install --global @jnyross/skillet@next`.

Building from source requires Go 1.25.0 or later (see `go.mod`).

## Usage

Run:

```
skillet
```

Skillet resolves everything it needs from your home directory: Claude Code
state under `~/.claude`, Codex state under `~/.codex` and `~/.agents`, and
its own data (the Archive) under `~/.skillet`. Project skills are discovered
from the current working directory up to the repository root (`ADR 0003`).
There is nothing to configure.

### Scriptable commands

Everything the TUI can do is also available non-interactively, so an agent
session or a CI job can manage Skills without a terminal UI:

```
skillet list [--json] [--source SOURCE] [--tool TOOL]
skillet show <name> [--json]
skillet cost [--json]
skillet archive <name> --yes
skillet restore <id|name> --yes
skillet purge <id|name> --yes
skillet suppress|unsuppress <name> --yes
skillet manual-only <name>... --yes
skillet manual-only --all [--except NAME[,NAME...]] --yes
skillet auto <name>... --yes
skillet library list|add|remove [--json]
skillet bundle list|install <bundle> --target <personal|PATH> [--json]
skillet install <library-entry> --target <personal|PATH> --yes
skillet setup [flags]
```

A bare Skill name works when it is unambiguous; otherwise Skillet exits 1 and
lists the qualified `Source:Name` candidates, which are themselves valid input.
Anything the TUI would confirm requires `--yes`, every mutation prints a
one-line summary of what changed, and the exit codes are 0 for success, 1 for
an operation error, and 2 for a usage error. `skillet --help` prints the full
command tree.

`skillet cost` estimates what your installed Skills cost in context: the
standing per-session cost of Auto-activation broken down per Tool, and the ten
Skills responsible for most of it. Every number it prints is an estimate.

`skillet manual-only` and `skillet auto` take any number of Skill names, and
`manual-only` also takes `--all` with an `--except` list — so one command gets
a machine down to the bare minimum set of Skills that auto-activate:

```
$ skillet manual-only --all --except code-review,handoff --yes
```

Without `--yes` it prints exactly what it would change and the estimated
per-session saving, and changes nothing. An ambiguous name aborts the whole
command before any write rather than half-applying it. When some Skills cannot
be changed — a Plugin Skill, whose control is Suppress, or a Codex prompt,
which has no Auto-activation to turn off — the rest are still applied, each
failure is explained, and the command exits 1.

`docs/agents/cli.md` documents the command tree, the name-resolution rules, and
the stable JSON schema (including how it stays additive).

### Main view

The main view lists every Skill Skillet found, grouped by Source —
**Personal**, **Plugin**, **Codex**, and **Project**. Project rows identify
whether Claude Code or Codex governs them. Each row shows the Skill's name and its
current Activation state (Auto, Manual-only, Suppressed, or Disabled); the full
description appears in the detail pane. A
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
| `pgup` / `pgdown` | Page the list | main view |
| `home` / `end` | Jump to the first / last Skill | main view |
| `ctrl+u` / `ctrl+d` (also `ctrl+pgup` / `ctrl+pgdown`) | Scroll the detail pane | main view |
| `/` | Filter the list | every view |
| `c` | Rank the list by estimated cost per session (press again to group by Source) | main view |
| `esc` | Clear the filter, then the marks, then return to the main view | every view |
| `space` | Mark / unmark the selected Skill | main view |
| `M` | Set every marked Skill to Manual-only | main view |
| `u` | Archive the selected Skill | Personal, Codex (skills and prompts), Project |
| `s` | Suppress / un-suppress the selected Skill | Plugin skills, Codex skills, Project Codex skills |
| `m` | Toggle Manual-only / Auto-activation | Personal skills, Codex skills (not prompts), Project skills |
| `x` | Uninstall the selected Skill's whole plugin | Plugin skills |
| `a` | Switch to the Archive view | main view |
| `o` | Open More: Library, Bundles, and Setup | main view |
| `l` | Add/remove the selected Skill or plugin from Library membership | supported installed entries |
| `L` | Switch to the Library view | main view |
| `B` | Switch to the Bundle view | main view |
| `S` | Setup workspace | main view |
| `?` | Show/hide all keys | everywhere |
| `q` / `ctrl+c` | Quit | everywhere except an open confirmation |

Pressing a key for an action that doesn't apply to the selected row (for
example `x` on a Personal skill) shows a status message explaining why,
instead of doing anything.

`ctrl+c` quits from anywhere except an open confirmation or picker prompt,
where it cancels the prompt instead of quitting. `esc` cancels whatever
overlay is open, and in the Library entry form it asks before discarding
anything you have already typed; `shift+tab` steps back to the previous field.

### What Skills cost you

Context is finite, and an Auto-activating Skill's description is injected into
**every** session with its Tool whether the Skill is ever used or not. Skillet
puts a number on that.

The line under the title is the total: what Auto-activation costs per session,
per Tool. Only Auto Skills count towards it — a Manual-only, Disabled, or
Suppressed Skill is not offered to the model unprompted, so it costs nothing
until you invoke it, and the line says how many Skills were excluded on that
basis.

The detail pane breaks the selected Skill's cost into the standing per-session
cost of its description, what invoking it costs (its whole `SKILL.md`), and
what it occupies on disk including references and scripts. Press `c` to rank
the whole list by per-session cost, most expensive first, across every Source;
press it again to go back to the Source grouping.

Every one of these numbers is an **estimate**. Skillet sizes files at roughly
four bytes per token rather than running a tokenizer, so treat them as a
reliable ranking rather than a measurement — they are for finding what to turn
off, not for budgeting to the token.

### Filtering

Press `/` in any view to filter the list, then type. Matching is fuzzy and
runs against the Skill's name, its description, its Source, its Tool, and the
plugin it came from — so `codex`, `review`, or part of a plugin name all
narrow the list. `enter` keeps the filter and returns to the list; `esc`
clears it. If nothing matches, Skillet says so instead of showing an empty
list. The Archive, Library, and Bundle views filter the same way, over their
own fields (original location, Library source, Bundle member names).

### Getting down to a minimum set

Every Auto-activating Skill's description is injected into every session with
its Tool, whether or not you use it. That standing cost is the number in the
header, and cutting it is what Skillet is for.

The fastest route is `c` to rank the list by estimated cost, then `space` to
mark the Skills you don't want deciding for themselves, then `M`. Skillet
confirms with the count and what it saves — `Set 31 Skills to Manual-only?
Saves ~1,940 tokens per session` — before writing anything. Manual-only is
not disabled: those Skills still run, they just wait to be asked. Marks
survive filtering, so you can filter, mark, clear the filter, and mark more
before acting. `esc` clears the marks.

The same sweep from a script, which is the one-liner for a fresh machine:

```sh
skillet manual-only --all --except code-review,handoff --yes
```

Without `--yes` it prints exactly what it would change and what it would
save, and changes nothing.

Library, Bundles, and guided Setup all still exist; they live behind `o`
(More) so the main view stays about seeing and reducing what you already
have.

### Archive view

Press `a` from the main view to see everything currently archived: name,
original Source, original location, and when it was archived.

| Key | Action |
|---|---|
| `up` / `k`, `down` / `j` | Move the cursor |
| `/` | Filter the Archive |
| `r` | Restore the selected entry |
| `p` | Purge the selected entry |
| `a` / `esc` | Back to the main view |
| `q` / `ctrl+c` | Quit |

### Confirmation model

Every action that changes something on disk asks for confirmation first:
Skillet shows a one-line description of exactly what it's about to do and
waits for a keypress. Press `y` to proceed; any other key cancels and
changes nothing. That covers Archive, Restore, Purge, Suppress, Manual-only,
Uninstall plugin, Install, adding and removing Library entries (including the
`l` toggle in the main view), creating and deleting Bundles, and adding,
removing, or re-activating a Bundle member. Browsing the list, moving the
cursor, filtering, and switching views never change anything by themselves —
only a confirmed `y` does.

### Library and Bundles

Library is a catalog of source pointers, not frozen copies. Press `L` to browse
it, `n` to add a local-path, git, skills.sh, or marketplace entry, `i` to
install the selected entry to Personal or a resolved Project, and `d` to remove
only the catalog record. Install resolves the current source each time.

| Key | Action |
|---|---|
| `up` / `k`, `down` / `j` | Move the cursor |
| `/` | Filter the Library |
| `i` | Install the selected entry |
| `n` | Add a new entry |
| `d` | Remove the selected entry from the Library |
| `L` / `esc` | Back to the main view |
| `q` / `ctrl+c` | Quit |

Press `B` for Bundles: named groups of Library entries with a remembered Auto
or Manual-only preference per member. Existing destinations are listed for
confirmation before replacement.

| Key | Action |
|---|---|
| `up` / `k`, `down` / `j` | Move the cursor |
| `/` | Filter the Bundles |
| `enter` / `space` | Expand or collapse the selected Bundle |
| `n` | Create a Bundle |
| `a` | Add a Library entry as a member |
| `r` | Remove the selected member |
| `m` | Toggle the selected member between Auto and Manual-only |
| `i` | Install the selected Bundle |
| `d` | Delete the selected Bundle |
| `B` / `esc` | Back to the main view |
| `q` / `ctrl+c` | Quit |

### Guided Project setup

Guided setup is available with either `skillet setup` or
`S` from the main inventory. Both entry points use the same setup service. The
main-TUI action offers the native macOS folder picker and falls back to guarded
terminal path entry if the picker is unavailable; the direct command starts at
the terminal path flow. Root, home, files, and nested non-root repository paths
are rejected before mutation.

**Guided Setup is Unix-only for v0.1.0.** Running `skillet setup` on Windows
returns a clear error before any mutation.

The Built-in catalog version `2026.07.15.2` contains 48 exact source boundaries:
21 Matt Pocock engineering/collaboration skills, the coherent 14-skill
Superpowers workflow, four Vercel frontend skills, three Apache-2.0 Anthropic
creator/UI skills, and six official .NET starter skills. Matt and Superpowers
remain separately selectable and selecting overlapping workflows produces an
explicit warning. The .NET Bundle is opt-in and discloses its SDK, diagnostic,
test, and network prerequisites. Setup never invokes those workflows or
installs their dependencies.

The reviewed Vercel revision declares MIT in its README but contains no license
grant text. Those four entries remain visible evidence but selection is blocked
until the maintainer resolves the notice policy; Skillet does not invent a
license notice. Every other member carries a hashed upstream MIT or Apache-2.0
license text. Latest source is resolved before setup. A byte-identical selected
boundary at a newer revision proceeds; material boundary, content, metadata,
license, dependency, external-action, script, or executable-mode drift is
shown and requires explicit consent.

Setup shows the normalized target, Git state, every Tool destination,
Activation, dependency readiness, and every Managed-file change before final
confirmation. Exact unmanaged content may be adopted. Conflicting or
user-edited Managed content blocks until the user authorizes a named backup and
replacement; required content is never skipped. An unchanged rerun is a no-op,
while Bundle removals and Activation changes are explicit and recoverable.
Use `--manual-only name-a,name-b` or `--auto name-c,name-d` with the direct
command, or enter member names at the interactive activation prompts, to
override both Tool views before the plan is applied.

The portable `.skillet/workspace.json` records catalog, Bundle, source,
Activation, Managed paths, hashes, drift decisions, and outcome. The ignored
`.skillet/workspace.local.json` stores machine-local executable, authentication,
dependency, and fresh-session probe results. Outcomes are exact:

- `Blocked`: no Managed change remains.
- `Configured-unverified`: both Tool views are statically valid, but an
  executable, authentication, or fresh-session discovery proof is missing.
- `Verified`: fresh Claude Code and Codex sessions discovered every selected
  member; optional workflow dependencies are still reported separately.
- `Partial`: an external Tool change could not be reversed and one exact repair
  action is provided.

Setup initializes Git when needed but never stages, commits, creates a remote,
generates application/framework content, installs SDKs or Tools, authenticates
accounts, runs member scripts, or silently polls for catalog updates.

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

Skillet covers Personal, Plugin, Codex, and Project skills, Codex custom
prompts, Library entries from local-path/git/skills.sh/marketplace sources,
and Bundles installed to Personal or a resolved Project.
