# Skillet's scriptable command surface

Everything the TUI can do, `skillet` can also do non-interactively, so an agent
session (Claude Code or Codex) or a CI job can manage Skills without a
terminal UI. `skillet` with no arguments still launches the TUI.

The vocabulary here is CONTEXT.md's: **Source**, **Tool**, **Skill**,
**Archive**, **Restore**, **Purge**, **Suppress**, **Manual-only**,
**Library**, **Bundle**, **Install**. JSON field names use the same terms.

## Command tree

```
skillet                                              launch the TUI
skillet --help | -h | help                           the full command tree
skillet version | --version                          release identity

skillet list [--json] [--source SOURCE] [--tool TOOL]
skillet show <name> [--json]

skillet archive <name> --yes
skillet restore <id|name> --yes
skillet purge <id|name> --yes

skillet suppress <name> --yes
skillet unsuppress <name> --yes
skillet manual-only <name> --yes
skillet auto <name> --yes

skillet library list [--json]
skillet library add --name NAME <source flags> [--tool TOOL] [--kind KIND] --yes [--json]
skillet library remove <id|name> --yes

skillet bundle list [--json]
skillet bundle install <id|name> --target <personal|PATH> --yes [--json]

skillet install <id|name> --target <personal|PATH> [--activation auto|manual-only] --yes

skillet setup [--path PATH] [--bundles IDS] [--manual-only MEMBERS] [--auto MEMBERS]
              [--yes] [--accept-drift] [--replace-conflicts] [--static]
```

Every subcommand accepts `--help`. Flags may appear before or after the
positional argument.

`library add` takes exactly one install-source descriptor:

- `--local-path PATH`
- `--git-url URL` with optional `--git-ref REF` and `--git-subpath PATH`
- `--skills-sh OWNER/REPO` with optional `--skills-sh-skill NAME`
- `--marketplace NAME --plugin NAME` with optional `--marketplace-source SOURCE`

## Naming and resolution

A bare Skill name works when exactly one Skill has it. When it does not,
Skillet exits 1 and prints the qualified candidates, each of which is itself
valid input:

```
$ skillet show review
"review" matches 2 Skills — re-run with one of:
  Personal:review  (/Users/me/.claude/skills/review)
  Project:review   (/Users/me/repo/.claude/skills/review)
```

The qualified forms are:

- `Source:Name` — for example `Personal:review`. Sources are `Personal`,
  `Plugin`, `Codex`, `Project` (case-insensitive).
- `Source:Tool:Name` — for example `Project:codex:review`. Needed only when
  one Source holds that name under both Tools. Tools are `claude-code` and
  `codex` (`claude` and `claude code` are also accepted).

`restore`, `purge`, `library remove`, `bundle install`, and `install` accept an
id or, when unambiguous, a name; ambiguity lists the ids.

## Exit codes and confirmation

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Operation error (no such Skill, ambiguous name, a write failed) |
| 2 | Usage error (unknown command or flag, missing argument, missing `--yes`) |

Anything the TUI would confirm requires `--yes` on the CLI, and every mutation
prints a one-line summary of exactly what changed:

```
$ skillet archive writing --yes
Archived Personal skill "writing" from /Users/me/.claude/skills/writing (archive id 1784924950240492000-writing; restore with "skillet restore 1784924950240492000-writing --yes")
```

`purge` is the only destructive command; without `--yes` it refuses and exits 2
without touching the Archive.

## JSON schema

Every JSON document is an object with `schemaVersion` (currently `1`).
`schemaVersion` changes only when an existing field is removed, renamed, or
given a new meaning — **new fields are added without bumping it**, so consumers
must ignore fields they do not know.

### `skillet list --json`

```json
{
  "schemaVersion": 1,
  "skills": [
    {
      "name": "lint",
      "qualifiedName": "Plugin:lint",
      "description": "Lint the tree.",
      "source": "Plugin",
      "tool": "Claude Code",
      "kind": "skill",
      "activation": "Auto",
      "location": "/Users/me/.claude/plugins/cache/demo/skills/lint",
      "declaredManualOnlyForClaude": false,
      "plugin": { "plugin": "demo", "marketplace": "market", "skillCount": 1 }
    }
  ],
  "notices": [
    { "message": "Codex skills directory not found: /Users/me/.codex/skills" }
  ],
  "archive": [
    {
      "id": "1784924950240492000-writing",
      "name": "writing",
      "source": "Personal",
      "tool": "Claude Code",
      "kind": "skill",
      "originalLocation": "/Users/me/.claude/skills/writing",
      "archivedAt": "2026-07-24T20:29:10Z"
    }
  ]
}
```

Skill fields:

| Field | Meaning |
|---|---|
| `name` | The Skill's name as its Tool sees it |
| `qualifiedName` | `Source:Name`, accepted as input by every command |
| `description` | The description the model reads when deciding to auto-activate |
| `source` | `Personal`, `Plugin`, `Codex`, or `Project` |
| `tool` | `Claude Code` or `Codex` |
| `kind` | `skill` or `prompt` |
| `activation` | `Auto`, `Manual-only`, `Suppressed`, or `Disabled` |
| `location` | Absolute path to the Skill directory (or prompt file) |
| `declaredManualOnlyForClaude` | A Codex Skill declaring Claude Code's `disable-model-invocation`, which Codex ignores |
| `plugin` | Present only for Plugin Skills: `plugin`, `marketplace`, `skillCount` |

`notices` carries every scan notice, and `archive` every visible Archive entry
— that is where `purge` and `restore` ids come from.

`--source` and `--tool` filter `skills`; `notices` and `archive` are not
filtered.

### `skillet show <name> --json`

```json
{ "schemaVersion": 1, "skill": { "...": "the same Skill object as list" } }
```

### `skillet library list --json` / `library add --json`

```json
{
  "schemaVersion": 1,
  "entries": [
    {
      "id": "1784924962760388000-helper",
      "name": "helper",
      "tool": "Claude Code",
      "source": { "kind": "local-path", "localPath": "/Users/me/src/helper" },
      "addedAt": "2026-07-24T20:29:22.760405Z"
    }
  ]
}
```

`library add --json` emits the created entry as `{"schemaVersion":1,"entry":{…}}`.
`source.kind` is one of `local-path`, `git`, `skills.sh`, `marketplace`, and the
descriptor fields present depend on the kind (`gitUrl`/`gitRef`/`gitSubPath`,
`skillsShRepo`/`skillsShSkill`, `marketplace`/`pluginName`).

### `skillet bundle list --json`

```json
{
  "schemaVersion": 1,
  "bundles": [
    {
      "id": "1784924962760388000-starter",
      "name": "starter",
      "members": [
        { "libraryEntryId": "1784924962760388000-helper", "activation": "Manual-only" }
      ]
    }
  ]
}
```

### `skillet bundle install --json`

```json
{
  "schemaVersion": 1,
  "bundle": { "id": "…", "name": "starter", "members": [] },
  "target": { "kind": "project", "repoRoot": "/Users/me/repo" },
  "installed": 1
}
```

## Recipes

```bash
# Everything the model can auto-activate today, by Tool.
skillet list --json | jq -r '.skills[] | select(.activation == "Auto") | "\(.tool)\t\(.qualifiedName)"'

# Make every Personal Skill Manual-only.
skillet list --json --source Personal |
  jq -r '.skills[].qualifiedName' |
  xargs -n1 -I{} skillet manual-only {} --yes

# Fail a CI job when scanning raised any notice.
test "$(skillet list --json | jq '.notices | length')" -eq 0

# Archive a Skill, then put it back.
skillet archive writing --yes
skillet restore "$(skillet list --json | jq -r '.archive[0].id')" --yes
```

## Notes and limits

- Project Sources are discovered from the current working directory, exactly as
  in the TUI, so `list` inside a repository shows that repository's Skills.
- `--target PATH` installs into that repository (`.claude/skills` or
  `.agents/skills` depending on the entry's Tool); `--target personal` installs
  at the user level. An existing Skill of the same name at the target is
  replaced.
- `suppress` applies to Plugin and Codex Skills; `manual-only`/`auto` apply to
  Personal, Project, and Codex Skills. Asking for one where the Source has no
  such mechanism is an operation error (exit 1) with an explicit message.
- `archive` covers Personal, Codex, and Project Skills and Codex prompts.
  A Plugin Skill cannot be archived alone — Suppress it instead.
